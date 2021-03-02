package util

import (
	"encoding/json"

	flyfish "github.com/sniperHW/flyfish/client"
)

func UnmarshalField(field *flyfish.Field, obj interface{}) error {
	if field == nil || field.GetBlob() == nil {
		return nil
	}
	return json.Unmarshal(field.GetBlob(), obj)
}
