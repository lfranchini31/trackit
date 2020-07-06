package models

import (
	"time"
)

// MostUsedTagsByAwsAccountIDInRange returns most used tags of an AWS account in a specified range.
func MostUsedTagsByAwsAccountIDInRange(db XODB, awsAccountID int, begin time.Time, end time.Time) ([]*MostUsedTag, error) {
	var err error

	// sql query
	sqlstr := `SELECT ` +
		`id, report_date, aws_account_id, tags ` +
		`FROM trackit.most_used_tags ` +
		`WHERE aws_account_id = ? ` +
		`AND report_date >= '` + begin.String() + `' ` +
		`AND report_date < '` + end.String() + `'`

	// run query
	XOLog(sqlstr, awsAccountID)
	q, err := db.Query(sqlstr, awsAccountID)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	// load results
	res := []*MostUsedTag{}
	for q.Next() {
		mut := MostUsedTag{
			_exists: true,
		}

		// scan
		err = q.Scan(&mut.ID, &mut.ReportDate, &mut.AwsAccountID, &mut.Tags)
		if err != nil {
			return nil, err
		}

		res = append(res, &mut)
	}

	return res, nil
}

// LatestMostUsedTagsByAwsAccountID returns the latest most used tags of an AWS account.
func LatestMostUsedTagsByAwsAccountID(db XODB, awsAccountID int, begin time.Time, end time.Time) (*MostUsedTag, error) {
	var err error

	// sql query
	sqlstr := `SELECT ` +
		`id, report_date, aws_account_id, tags ` +
		`FROM trackit.most_used_tags ` +
		`WHERE aws_account_id = ? ` +
		`ORDER BY report_date DESC LIMIT 1`

	// run query
	XOLog(sqlstr, awsAccountID)
	q, err := db.Query(sqlstr, awsAccountID)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	// load results
	res := []*MostUsedTag{}
	for q.Next() {
		mut := MostUsedTag{
			_exists: true,
		}

		// scan
		err = q.Scan(&mut.ID, &mut.ReportDate, &mut.AwsAccountID, &mut.Tags)
		if err != nil {
			return nil, err
		}

		res = append(res, &mut)
	}

	if len(res) <= 0 {
		return nil, nil
	}

	return res[0], nil
}
