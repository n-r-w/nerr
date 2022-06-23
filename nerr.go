package nerr

import (
	"errors"
	"fmt"
	"runtime"
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
	return e.Err
}

func (e *Error) Ops() []string {
	return Ops(e)
}

func (e *Error) TopCode() int {
	return TopCode(e)
}

func (e *Error) Trace() []string {
	return Trace(e)
}

func New(args ...interface{}) error {
	if len(args) == 1 && args[0] == nil {
		return nil
	}

	e := &Error{}

	if pc, file, line, ok := runtime.Caller(1); ok {
		details := runtime.FuncForPC(pc)
		e.Place = fmt.Sprintf("%s (%s:%d)", details.Name(), file, line)
	}

	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			if len(e.Op) > 0 {
				if len(v) > 0 {
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
		case error:
			if e.Err != nil {
				panic("error duplication")
			}
			e.Err = v
		default:
			panic(fmt.Sprintf("invalid argument type: %T", arg))
		}
	}

	return e
}

func NewFmt(format string, args ...interface{}) error {
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

func Trace(e error) []string {
	res := []string{}
	if e == nil {
		return res
	}

	switch v := e.(type) {
	case *Error:
		res = append(res, v.Place)
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
