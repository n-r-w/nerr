package nerr

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/jackc/pgconn"
	"github.com/lib/pq"
	"github.com/n-r-w/eno"
)

type Error struct {
	Op    string
	Code  int
	Place string
	Err   error
}

func (e *Error) Error() string {
	var res []string

	if len(e.Op) > 0 {
		res = append(res, fmt.Sprintf("op: %s", e.Op))
	}

	code := e.TopCode()
	if code > 0 {
		res = append(res, fmt.Sprintf("code: %d", code))
	}

	if trace := e.Trace(); len(trace) > 0 {
		res = append(res, fmt.Sprintf("source: %s", trace[len(trace)-1]))
	}

	if len(res) == 0 && e.Err == nil {
		return "undefined"
	}

	s := strings.Join(res, ", ")

	if e.Err != nil {
		if len(s) == 0 {
			s = e.Err.Error()
		} else {
			s = fmt.Sprintf("%s => %v", s, e.Err)
		}
	}

	return s
}

func (e *Error) Unwrap() error {
	if e.Err == nil {
		return e
	} else {
		return e.Err
	}
}

func (e *Error) Ops() []string {
	return Ops(e)
}

func (e *Error) TopCode() int {
	return TopCode(e)
}

func (e *Error) TopOp() string {
	return TopOp(e)
}

func (e *Error) Trace() []string {
	return Trace(e)
}

func New(args ...any) error {
	return NewLevel(2, args)
}

func NewLevel(codeLevel int, args ...any) error {
	if len(args) == 1 {
		if args[0] == nil {
			return nil
		}
		if e, ok := args[0].(error); ok && e == nil {
			return nil
		}
	}

	e := &Error{}

	if pc, file, line, ok := runtime.Caller(codeLevel); ok {
		details := runtime.FuncForPC(pc)
		e.Place = fmt.Sprintf("%s (%s:%d)", details.Name(), file, line)
	}

	for _, arg := range args {
		if !prepareProperty(e, arg) {
			return nil
		}

	}

	return e
}

func prepareProperty(e *Error, arg any) bool {
	if arg == nil {
		return false
	}

	switch v := arg.(type) {
	case string:
		if len(e.Op) > 0 {
			if len(v) > 0 && e.Op != v {
				e.Op += ", " + v
			}
		} else {
			e.Op = v
		}
	case eno.ErrNo:
		e.Code = int(v)
		if len(e.Op) == 0 {
			e.Op = eno.Name(v)
		}
	case int, int8, int32:
		if e.Code != 0 {
			panic("code duplication")
		}
		e.Code = v.(int)
	case []error:
		if len(v) == 1 {
			return prepareProperty(e, v[0])
		}

		var errs []string
		for _, e := range v {
			if e != nil {
				errs = append(errs, e.Error())
			}
		}

		if len(errs) > 0 {
			if e.Err != nil {
				panic("error duplication")
			}

			e.Err = errors.New(strings.Join(errs, ", "))
		} else {
			return false
		}
	case []any:
		var errs []string
		if len(v) == 1 {
			return prepareProperty(e, v[0])
		}

		for _, e := range v {
			if e != nil {
				text := fmt.Sprintf("%v", e)
				if len(text) > 0 {
					errs = append(errs, fmt.Sprintf("%v", e))
				}
			}
		}

		if len(errs) > 0 {
			if e.Err != nil {
				panic("error duplication")
			}

			e.Err = errors.New(strings.Join(errs, ", "))
		} else {
			return false
		}
	case error:
		if e.Err != nil {
			panic("error duplication")
		}
		e.Err = v

	default:
		panic(fmt.Sprintf("invalid argument type: %T", arg))
	}

	return true
}

func NewFmt(format string, args ...any) error {
	return New(fmt.Sprintf(format, args...))
}

func Ops(e error) []string {
	res := []string{}
	if e == nil {
		return res
	}

	switch v := e.(type) {
	case *Error:
		res = append(res, v.Op)
		if v.Err != nil {
			res = append(res, Ops(v.Err)...)
		}
		return res
	default:
		return []string{v.Error()}
	}
}

func TopCode(e error) int {
	if e == nil {
		return 0
	}

	switch v := e.(type) {
	case *Error:
		if v.Code != 0 {
			return int(v.Code)
		}
		return TopCode(v.Err)
	default:
		return 0
	}
}

func TopOp(e error) string {
	ops := Ops(e)
	if len(ops) > 0 {
		return ops[len(ops)-1]
	} else {
		return strconv.Itoa(TopCode(e))
	}
}

func Trace(e error) []string {
	res := []string{}
	if e == nil {
		return res
	}

	switch v := e.(type) {
	case *Error:
		info := []string{v.Place}
		if len(v.Op) > 0 {
			info = append(info, "op: "+v.Op)
		}
		if v.Code != 0 {
			info = append(info, fmt.Sprintf("code: %d", v.Code))
		}

		res = append(res, strings.Join(info, "; "))
		if v.Err != nil {
			res = append(res, Trace(v.Err)...)
		}
		return res
	default:
		return res
	}
}

func IsCode(err error, code int) bool {
	if err == nil {
		return false
	}

	c := TopCode(err)
	if c == code {
		return true
	}

	if e, ok := err.(*Error); ok && e.Err != nil {
		return IsCode(e.Err, code)
	}

	return false
}

func Is(err, target error) bool {
	return errors.Is(err, target)
}

func As(err error, target any) bool {
	return errors.As(err, target)
}

func Unwrap(err error) error {
	return errors.Unwrap(err)
}

// Сравнение кодов с помощью github.com/jackc/pgerrcode
func SqlCode(err error) string {
	e := Unwrap(err)

	if pqerr, ok := e.(*pq.Error); ok {
		return string(pqerr.Code)

	} else if pqerr, ok := e.(*pgconn.PgError); ok {
		return pqerr.Code
	}

	return ""
}
