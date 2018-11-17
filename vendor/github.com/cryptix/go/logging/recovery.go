package logging

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime/debug"

	"github.com/pkg/errors"
)

func RecoveryHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() {
				if r := recover(); r != nil {
					if err := LogPanicWithStack(FromContext(req.Context()), "httpRecovery", r); err != nil {
						fmt.Fprintf(os.Stderr, "PanicLog failed! %q", err)
						panic(err)
					}
					http.Error(w, "internal processing error - please try again", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, req)
		})
	}
}

func LogPanicWithStack(log Interface, location string, r interface{}) error {
	var err error
	switch t := r.(type) {
	case string:
		err = errors.New(t)
	case error:
		err = t
	default:
		err = errors.Errorf("unkown type(%T) error: %v", r, r)
	}
	os.Mkdir("panics", os.ModePerm)
	b, tmpErr := ioutil.TempFile("panics", location)
	if tmpErr != nil {
		log.Log("event", "panic", "location", location, "err", err, "warning", "no temp file", "tmperr", tmpErr)
		return errors.Wrapf(tmpErr, "LogPanic: failed to create httpRecovery log")
	}

	fmt.Fprintf(b, "warning! %s!\nError: %s\n", location, err)
	fmt.Fprintf(b, "Stack:\n%s", debug.Stack())

	log.Log("event", "panic", "location", location, "panicLog", b.Name())

	return errors.Wrap(b.Close(), "LogPanic: failed to close dump file")
}
