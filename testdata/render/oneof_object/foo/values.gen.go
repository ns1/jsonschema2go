package foo

import (
	"encoding/json"
	"fmt"
)

// Bar gives you some dumb info
type Bar struct {
	Direction interface{}
}

func (m *Bar) UnmarshalJSON(data []byte) error {
	var discrim struct {
		Direction string `json:"direction"`
	}
	if err := json.Unmarshal(data, &discrim); err != nil {
		return err
	}
	switch discrim.Direction {
	case "l":
		m.Direction = new(Left)
	case "r":
		m.Direction = new(Right)
	default:
		return fmt.Errorf("unknown discriminator: %v", discrim.Direction)
	}
	return json.Unmarshal(data, m.Direction)
}

func (m *Bar) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Direction)
}

type Left struct {
	Direction string `json:"direction,omitempty"`
	Value     int    `json:"value,omitempty"`
}

type Right struct {
	Direction string  `json:"direction,omitempty"`
	Value     float64 `json:"value,omitempty"`
}
