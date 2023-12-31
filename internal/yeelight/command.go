package yeelight

import (
	"encoding/json"

	"github.com/cybre/yeelight-controller/internal/errors"
)

type command struct {
	ID     int           `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

func newCommand(id int, method string, params ...interface{}) command {
	return command{
		ID:     id,
		Method: method,
		Params: params,
	}
}

func (c *command) String() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal bulb command")
	}

	return string(b) + "\r\n", err
}
