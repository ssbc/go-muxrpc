package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
  "context"
  "encoding/json"
  "reflect"

  "cryptoscope.co/go/luigi"
  "cryptoscope.co/go/luigi/mfr"
)

func NewDecoder(src luigi.Source, tipe interface{}) luigi.Source {
  t := reflect.TypeOf(tipe)
  if t.Kind() == reflect.Ptr {
    t = t.Elem()
  }

  return mfr.SourceMap(src, func(ctx context.Context, v interface{}) (interface{}, error) {
    data := v.([]byte)

    dst := reflect.New(t).Interface()

    err := json.Unmarshal(data, dst)

    return reflect.ValueOf(dst).Elem().Interface(), err
  })
}

