package db

import (
	"encoding/json"
	"fmt"
)

type CabinetImage struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Alt       string `json:"alt"`
	SortOrder int32  `json:"sort_order"`
}

type CabinetImages []CabinetImage

func (imgs *CabinetImages) Scan(src any) error {
	if src == nil {
		*imgs = CabinetImages{}
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into CabinetImages", src)
	}
	if len(b) == 0 {
		*imgs = CabinetImages{}
		return nil
	}
	return json.Unmarshal(b, imgs)
}
