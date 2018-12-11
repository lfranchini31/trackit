//   Copyright 2018 MSolution.IO
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package reservedInstances

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/trackit/trackit-server/aws/usageReports"
)

func getRecurringCharges(reservation *ec2.ReservedInstances) []RecurringCharges {
	charges := make([]RecurringCharges, len(reservation.RecurringCharges))
	for i, key := range reservation.RecurringCharges {
		charges[i] = RecurringCharges{
			Amount:    aws.Float64Value(key.Amount),
			Frequency: aws.StringValue(key.Frequency),
		}
	}
	return charges
}

// getReservationTag formats []*ec2.Tag to map[string]string
func getReservationTag(tags []*ec2.Tag) []utils.Tag {
	res := make([]utils.Tag, 0)
	for _, tag := range tags {
		res = append(res, utils.Tag{
			Key:   aws.StringValue(tag.Key),
			Value: aws.StringValue(tag.Value),
		})
	}
	return res
}
