/*
muxgen generates code to easily call functions on the endpoint

type myEndpoint struct {
 // ...
}

// go:generate muxgen -outtype []byte *myEndpoint async whoami
// go:generate muxgen -args id:string *myEndpoint source getFeedStream
// go:generate muxgen -args id:string *myEndpoint source blobs.get
// go:generate muxgen -args id:string -outtype bool *myEndpoint aysnc blobs.has
*/
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"

	"go.cryptoscope.co/muxrpc"

	"github.com/pkg/errors"
)

type Arg struct {
	name string
	tipe string
}

func parseArg(arg string) (Arg, error) {
	split := strings.Split(arg, ":")
	if len(split) != 2 {
		return Arg{}, errors.Errorf("not exactly one : in %q", arg)
	}

	return Arg{name: split[0], tipe: split[1]}, nil
}

type Args []Arg

func parseArgs(args string) (Args, error) {
	if args == "" {
		return Args{}, nil
	}

	split := strings.Split(args, ",")
	out := make(Args, len(split))

	var err error
	for i := range split {
		out[i], err = parseArg(split[i])
		if err != nil {
			return nil, errors.Wrap(err, "error parsing arg")
		}
	}

	return out, nil
}

func (args Args) Signature() string {
	arglist := make([]string, len(args)+1)

	arglist[0] = "ctx context.Context"
	for i, arg := range args {
		arglist[i+1] = arg.name + " " + arg.tipe
	}

	return strings.Join(arglist, ", ")
}

func (args Args) AsArgs() string {
	arglist := make([]string, len(args))

	for i, arg := range args {
		arglist[i] = arg.name
	}

	return strings.Join(arglist, ", ")
}

type Func struct {
	Receiver string
	Type     muxrpc.CallType
	Method   muxrpc.Method
	Args     Args
	OutType  string
}

func (f *Func) Name() string {
	var name string

	for _, el := range f.Method {
		name += strings.Title(el)
	}

	return name
}

func Parse(args []string) (*Func, error) {
	var (
		arglist string
		outType string
	)

	set := flag.NewFlagSet("muxgen", flag.ContinueOnError)
	set.StringVar(&arglist, "args", "", "arguments of the call. comma seperated name:type pairs, e.g.: id:string,count:int")
	set.StringVar(&outType, "outtype", "", "type of the values we expect to receive, either from async or source call")

	err := set.Parse(args)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing flags")
	}

	args = set.Args()

	if len(args) != 3 {
		return nil, errors.Errorf("expected 3 args, got %d", len(args))
	}

	m := &Func{
		Receiver: args[0],
		Type:     muxrpc.CallType(args[1]),
		Method:   strings.Split(args[2], "."),
		OutType:  outType,
	}

	m.Args, err = parseArgs(arglist)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing arguments")
	}

	return m, nil
}

const (
	asyncTpl = `{{ "" -}}
func (edp {{ .Receiver }}) {{ .Name }}({{ .Args.Signature }}) ({{ .OutType }}, error) {
	var tipe {{ .OutType }}
	v, err := edp.Endpoint.Async(ctx, tipe, {{ printf "%#v" .Method }} {{- with .Args.AsArgs}}, {{ . }} {{- end}})
	if err != nil {
		return nil, errors.Wrap(err, "error in async call to {{ .Name }}")
	}

	return v.({{.OutType}}), nil
}`

	sourceTpl = `{{ "" -}}
func (edp {{ .Receiver }}) {{ .Name }}({{ .Args.Signature }}) (luigi.Source, error) {
	var tipe {{ .OutType }}
	src, err := edp.Endpoint.Source(ctx, tipe, {{ printf "%#v" .Method }} {{- with .Args.AsArgs}}, {{ . }} {{- end}})
	if err != nil {
		return nil, errors.Wrap(err, "error in source call to {{ .Name }}")
	}

	return src, nil
}`

	sinkTpl = `{{ "" -}}
func (edp {{ .Receiver }}) {{ .Name }}({{ .Args.Signature }}) (luigi.Sink, error) {
	sink, err := edp.Endpoint.Sink(ctx, {{ printf "%#v" .Method }} {{- with .Args.AsArgs}}, {{ . }} {{- end}})
	if err != nil {
		return nil, errors.Wrap(err, "error in sink call to {{ .Name }}")
	}

	return sink, nil
}`

	duplexTpl = `{{ "" -}}
func (edp {{ .Receiver }}) {{ .Name }}({{ .Args.Signature }}) (luigi.Source, luigi.Sink, error) {
	var tipe {{ .OutType }}
	src, sink, err := edp.Endpoint.Duplex(ctx, tipe, {{ printf "%#v" .Method }} {{- with .Args.AsArgs}}, {{ . }} {{- end}})
	if err != nil {
		return nil, errors.Wrap(err, "error in sink call to {{ .Name }}")
	}

	return sink, nil
}`
)

func (m *Func) Generate() (string, error) {
	switch m.Type {
	case "async":
		return m.generateAsync()
	case "source":
		return m.generateSource()
	case "sink":
		return m.generateSink()
	case "duplex":
		return m.generateDuplex()
	default:
		return "", errors.Errorf("unknown call type %q", m.Type)
	}
}

func (m *Func) generateAsync() (string, error) {
	tpl := template.Must(template.New("").Parse(asyncTpl))
	var b bytes.Buffer

	err := tpl.Execute(&b, m)
	return b.String(), err
}

func (m *Func) generateSource() (string, error) {
	tpl := template.Must(template.New("").Parse(sourceTpl))
	var b bytes.Buffer

	err := tpl.Execute(&b, m)
	return b.String(), err
}

func (m *Func) generateSink() (string, error) {
	tpl := template.Must(template.New("").Parse(sinkTpl))
	var b bytes.Buffer

	err := tpl.Execute(&b, m)
	return b.String(), err
}

func (m *Func) generateDuplex() (string, error) {
	tpl := template.Must(template.New("").Parse(duplexTpl))
	var b bytes.Buffer

	err := tpl.Execute(&b, m)
	return b.String(), err
}

func closeErr(err error) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "an error occurred: %+v\n", err)
	os.Exit(1)
}

func main() {
	m, err := Parse(os.Args[1:])
	closeErr(errors.Wrap(err, "error parsing command line"))

	out, err := m.Generate()
	closeErr(errors.Wrap(err, "error generating output"))

	fmt.Println(out)
}
