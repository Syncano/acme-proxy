package server

import "net/url"

type Route struct {
	Path    string
	Headers map[string]string
	Targets []*YAMLURL
}

type YAMLURL struct {
	*url.URL
}

func (j *YAMLURL) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string

	err := unmarshal(&s)
	if err != nil {
		return err
	}

	urll, err := url.Parse(s)
	j.URL = urll

	return err
}
