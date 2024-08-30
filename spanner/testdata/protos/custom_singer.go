package protos

import (
	"errors"
)

func (c *CustomSingerInfo) EncodeSpanner() (interface{}, error) {
	if c == nil {
		return nil, nil
	}
	return c.SingerName, nil
}

func (c CustomGenre) EncodeSpanner() (interface{}, error) {
	return c.String(), nil
}

func (c *CustomSingerInfo) DecodeSpanner(input interface{}) error {
	str, ok := input.(string)
	if !ok {
		return errors.New("the interface does not contain a string")
	}
	c.SingerName = &str
	return nil
}
