/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
