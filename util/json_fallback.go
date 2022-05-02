//go:build !amd64
// +build !amd64

package util

import jsoniter "github.com/json-iterator/go"

var jsoniterconfiged = jsoniter.ConfigDefault

func marshalJSON(v interface{}) ([]byte, error) {
	return jsoniterconfiged.Marshal(v)
}

func unmarshalJSON(b []byte, v interface{}) error {
	return jsoniterconfiged.Unmarshal(b, v)
}

func marshalJSONIndent(i interface{}) ([]byte, error) {
	return jsoniterconfiged.MarshalIndent(i, "", "  ")
}