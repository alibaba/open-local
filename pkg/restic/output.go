package restic

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

const (
	PhaseOpString = "###Phase-output###:"
	reStr         = PhaseOpString + `(.*)$`
)

type Output struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// UnmarshalOutput unmarshals output json into Output struct
func UnmarshalOutput(opString string) (*Output, error) {
	p := &Output{}
	err := json.Unmarshal([]byte(opString), p)
	return p, errors.Wrap(err, "Failed to unmarshal key-value pair")
}

var logRE = regexp.MustCompile(reStr)

func Parse(l string) (*Output, error) {
	l = strings.TrimSpace(l)
	match := logRE.FindAllStringSubmatch(l, 1)
	if len(match) == 0 {
		return nil, nil
	}
	op, err := UnmarshalOutput(match[0][1])
	if err != nil {
		return nil, err
	}
	return op, nil
}
