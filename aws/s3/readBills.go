package s3

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/trackit/jsonlog"

	taws "github.com/trackit/trackit2/aws"
	"github.com/trackit/trackit2/config"
	"github.com/trackit/trackit2/util"
	"github.com/trackit/trackit2/util/csv"
)

const (
	// MaxCheckedKeysByRepository is the amount of keys inspected before we give
	// up. If users have a massive bucket where their bills are stored alongside
	// other keys, we don't to spend too much time reading the metadata of all
	// keys. This means that it is the responsibility of the user to put their
	// bills in a place where there isn't much of anything else.
	MaxCheckedKeysByRepository = 1000

	ReadBillsStsSessionName = "read-bills"
)

var (
	ErrUnsupportedCompression = errors.New("unsupported compression")
)

const maxManifestSize = 0x8000

type manifest struct {
	SourceBucket string   `json:"sourceBucket"`
	Bucket       string   `json:"bucket"`
	ReportKeys   []string `json:"reportKeys"`
	Compression  string   `json:"compression"`
	ReportName   string   `json:"reportName"`
	Account      string   `json:"account"`
}

// BillKey is a key where a bill object may be found.
type BillKey struct {
	Region       string
	Bucket       string
	Key          string
	LastModified time.Time
}

type billRepositoryWithRegion struct {
	BillRepository
	Region string `json:"region"`
}

type LineItem struct {
	LineItemId       string            `csv:"identity/LineItemId"`
	TimeInterval     string            `csv:"identity/TimeInterval"`
	InvoiceId        string            `csv:"bill/InvoiceId"`
	UsageAccountId   string            `csv:"lineItem/UsageAccountId"`
	UsageStartDate   string            `csv:"lineItem/UsageStartDate"`
	UsageEndDate     string            `csv:"lineItem/UsageEndDate"`
	ProductCode      string            `csv:"lineItem/ProductCode"`
	UsageType        string            `csv:"lineItem/UsageType"`
	Operation        string            `csv:"lineItem/Operation"`
	AvailabilityZone string            `csv:"lineItem/AvailabilityZone"`
	ResourceId       string            `csv:"lineItem/ResourceId"`
	CurrencyCode     string            `csv:"lineItem/CurrencyCode"`
	UnblendedCost    string            `csv:"lineItem/UnblendedCost"`
	Any              map[string]string `csv:",any"`
}

type OnLineItem func(LineItem) bool

func ReadBills(ctx context.Context, aa taws.AwsAccount, br BillRepository, oli OnLineItem) error {
	s3svc, brr, err := getServiceForRepository(ctx, aa, br)
	if err != nil {
		return err
	}
	jsonlog.LoggerFromContextOrDefault(ctx).Debug("Obtained S3 service to read bills.", map[string]interface{}{"account": aa, "billRepository": br})
	mc := getManifests(ctx, s3svc, getManifestKeys(ctx, getKeys(ctx, s3svc, brr)))
	importBills(ctx, s3svc, mc, oli)
	return nil
}

func importBills(ctx context.Context, s3svc *s3.S3, manifests <-chan manifest, oli OnLineItem) {
	l := jsonlog.LoggerFromContextOrDefault(ctx)
	out := make(chan LineItem)
	outs := make(chan (<-chan LineItem))
	go mergecLineItem(out, outs)
	for m := range manifests {
		if billIsInteresting(m) {
			l.Debug("Will attempt ingesting bills.", m)
			for _, s := range m.ReportKeys {
				l.Debug("Will attempt ingesting bill part.", map[string]interface{}{"key": s, "manifest": m})
				outs <- importBill(ctx, s3svc, s, m, oli)
			}
		} else {
			l.Debug("Reports do not interest us.", m)
		}
	}
}

func billIsInteresting(_ manifest) bool {
	return true
}

func importBill(ctx context.Context, s3svc *s3.S3, s string, m manifest, oli OnLineItem) <-chan LineItem {
	out := make(chan LineItem)
	go func() {
		defer close(out)
		ctx, cancel := context.WithCancel(ctx)
		l := jsonlog.LoggerFromContextOrDefault(ctx)
		reader, err := getBillReader(ctx, s3svc, s, m)
		if err != nil {
			l.Error("Failed to read bill.", err.Error())
		} else {
			l.Debug("Reading bill.", map[string]interface{}{"key": s, "manifest": m})
			readBill(ctx, cancel, reader, s, m, oli)
		}
	}()
	return out
}

func readBill(ctx context.Context, cancel context.CancelFunc, reader io.ReadCloser, s string, m manifest, oli OnLineItem) {
	defer reader.Close()
	csvDecoder := csv.NewDecoder(reader)
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	for r := range records(ctx, &csvDecoder) {
		if r.InvoiceId == "" {
			cancel()
			logger.Info("Canceled non-final report import.", map[string]interface{}{"key": s, "manifest": m})
			return
		}
		oli(r)
	}
}

func records(ctx context.Context, d *csv.Decoder) <-chan LineItem {
	out := make(chan LineItem)
	log := jsonlog.LoggerFromContextOrDefault(ctx)
	go func() {
		defer close(out)
		if err := d.ReadHeader(); err != nil {
			log.Error("Failed to read CSV header.", err.Error())
			return
		}
		for {
			record, err := decodeRecord(d)
			if err == io.EOF {
				return // EOF was reached
			} else if err != nil {
				log.Error("Error reading CSV record.", err.Error())
				return
			} else {
				select {
				case out <- record:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func decodeRecord(d *csv.Decoder) (LineItem, error) {
	var record LineItem
	err := d.ReadRecord(&record)
	return record, err
}

func getBillReader(ctx context.Context, s3svc *s3.S3, s string, m manifest) (io.ReadCloser, error) {
	switch m.Compression {
	case "GZIP":
		return getGzipBillReader(ctx, s3svc, s, m)
	default:
		jsonlog.LoggerFromContextOrDefault(ctx).Error("Unsupported  compression scheme.", map[string]interface{}{"key": s, "manifest": m})
		return nil, ErrUnsupportedCompression
	}
}

func getGzipBillReader(ctx context.Context, s3svc *s3.S3, s string, m manifest) (io.ReadCloser, error) {
	input := s3.GetObjectInput{
		Bucket: &m.Bucket,
		Key:    &s,
	}
	if output, err := s3svc.GetObjectWithContext(ctx, &input); err == nil {
		return gzip.NewReader(output.Body)
	} else {
		return nil, err
	}
}

func getManifests(ctx context.Context, s3svc *s3.S3, in <-chan BillKey) <-chan manifest {
	out := make(chan manifest)
	outs := make(chan (<-chan manifest))
	go mergecManifest(out, outs)
	go func() {
		defer close(outs)
		s3mgr := s3manager.NewDownloaderWithClient(s3svc)
		for bk := range in {
			outs <- readManifest(ctx, s3mgr, bk)
		}
	}()
	return out
}

// mergecLineItem implements the fan-in pattern by merging to the out
// channel the input from the channels read on cs.
func mergecLineItem(out chan<- LineItem, cs <-chan <-chan LineItem) {
	var wg sync.WaitGroup
	for c := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range c {
				out <- u
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
}

// mergecManifest implements the fan-in pattern by merging to the out
// channel the input from the channels read on cs.
func mergecManifest(out chan<- manifest, cs <-chan <-chan manifest) {
	var wg sync.WaitGroup
	for c := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range c {
				out <- u
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
}

func readManifest(ctx context.Context, s3mgr *s3manager.Downloader, bk BillKey) <-chan manifest {
	out := make(chan manifest)
	go func() {
		defer close(out)
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		buf := util.FixedBuffer(make([]byte, maxManifestSize))
		input := s3.GetObjectInput{
			Bucket: &bk.Bucket,
			Key:    &bk.Key,
		}
		n, err := s3mgr.DownloadWithContext(ctx, buf, &input)
		if err != nil {
			logger.Error("Failed to download usage and cost manifest.", map[string]interface{}{"billKey": bk, "error": err.Error()})
			return
		} else {
			var m manifest
			err := json.Unmarshal(buf[:n], &m)
			if err != nil {
				logger.Error("Failed to parse usage and cost manifest.", map[string]interface{}{"billKey": bk, "error": err.Error()})
				return
			} else {
				m.SourceBucket = bk.Bucket
				out <- m
			}
		}

	}()
	return out
}

// getServiceForRepository instantiates an *s3.S3 service from an AwsAccount
// and a billRepositoryWithRegion. It returns a nil error iff the operation was
// successful.
func getServiceForRepository(ctx context.Context, aa taws.AwsAccount, br BillRepository) (*s3.S3, billRepositoryWithRegion, error) {
	var brr billRepositoryWithRegion
	creds, err := taws.GetTemporaryCredentials(aa, ReadBillsStsSessionName)
	if err != nil {
		return nil, brr, err
	}
	jsonlog.LoggerFromContextOrDefault(ctx).Debug("Obtained credentials to read bills.", map[string]interface{}{"account": aa, "billRepository": br})
	sess := session.New(&aws.Config{Credentials: creds, Region: &config.AwsRegion})
	region, err := getBucketRegion(ctx, sess, br)
	if err != nil {
		return nil, brr, err
	}
	brr.BillRepository = br
	brr.Region = region
	return serviceForBucketRegion(sess, region), brr, nil
}

// getKeys returns a channel where all keys from the billRepositoryWithRegion
// will be sent.
func getKeys(ctx context.Context, s3svc *s3.S3, brr billRepositoryWithRegion) <-chan BillKey {
	c := make(chan BillKey)
	l := jsonlog.LoggerFromContextOrDefault(ctx)
	l.Debug("Getting manifest files from repository.", brr)
	go func() {
		defer close(c)
		input := s3.ListObjectsV2Input{
			Bucket: &brr.Bucket,
			Prefix: &brr.Prefix,
		}
		err := s3svc.ListObjectsV2PagesWithContext(ctx, &input, listBillsFromRepositoryPage(ctx, c, brr, l))
		if err != nil {
			l.Error("Failed to list objects from bucket.", err.Error())
		}
	}()
	return c
}

// manifestKeyRegex matches keys which look like manifest keys.
var manifestKeyRegex = regexp.MustCompile(`/\d{8}-\d{8}\/[^/]+-Manifest.json$`)

// getManifestKeys filters a channel of BillKey to only keep those which seem to
// be Cost And Usage manifests.
func getManifestKeys(ctx context.Context, in <-chan BillKey) <-chan BillKey {
	out := make(chan BillKey)
	l := jsonlog.LoggerFromContextOrDefault(ctx)
	go func() {
		defer close(out)
		for bk := range in {
			if manifestKeyRegex.MatchString(bk.Key) {
				l.Debug("Found manifest key.", map[string]interface{}{"billKey": bk})
				select {
				case out <- bk:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

// listBillsFromRepositoryPage handles a page of results for
// listBillsFromRepository. It will only trigger the processing of the next
// page if less than MaxCheckedKeysByRepository keys where encountered.
func listBillsFromRepositoryPage(
	ctx context.Context,
	c chan<- BillKey,
	brr billRepositoryWithRegion,
	l jsonlog.Logger,
) func(*s3.ListObjectsV2Output, bool) bool {
	count := 0
	return func(page *s3.ListObjectsV2Output, last bool) bool {
		for _, o := range page.Contents {
			select {
			case c <- BillKey{
				Key:          *o.Key,
				Bucket:       brr.Bucket,
				Region:       brr.Region,
				LastModified: *o.LastModified,
			}:
			case <-ctx.Done():
				return false
			}
		}
		count += len(page.Contents)
		if count < MaxCheckedKeysByRepository {
			return !last
		} else {
			l.Warning("Checked maximum amount of keys for repository.", brr)
			return false
		}
	}
}

// serviceForBucketRegion determines the region an S3 bucket resides in and
// returns that as a string.
func getBucketRegion(ctx context.Context, sess *session.Session, r BillRepository) (string, error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	s3svc := s3.New(sess)
	input := s3.GetBucketLocationInput{
		Bucket: &r.Bucket,
	}
	if output, err := s3svc.GetBucketLocationWithContext(ctx, &input); err == nil {
		region := *output.LocationConstraint
		logger.Debug(fmt.Sprintf("Found bucket region."), map[string]string{
			"bucket": r.Bucket,
			"region": region,
		})
		return region, nil
	} else {
		return "", err
	}
}

// serviceForBucketRegion returns an S3 service for the given region.
func serviceForBucketRegion(sess *session.Session, region string) *s3.S3 {
	return s3.New(sess.Copy(&aws.Config{Region: &region}))
}
