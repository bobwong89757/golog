// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log implements a simple logging package. It defines a type, Logger,
// with methods for formatting output. It also has a predefined 'standard'
// Logger accessible through helper functions Print[f|ln], Fatal[f|ln], and
// Panic[f|ln], which are easier to use than creating a Logger manually.
// That logger writes to standard error and prints the date and time
// of each logged message.
// The Fatal functions call os.Exit(1) after writing the log message.
// The Panic functions call panic after writing the log message.
package golog

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

// These flags define which text to prefix to each log entry generated by the Logger.
const (
	// Bits or'ed together to control what's printed. There is no control over the
	// order they appear (the order listed here) or the format they present (as
	// described in the comments).  A colon appears after these items:
	//	2009/01/23 01:23:23.123123 /a/b/c/d.go:23: message
	Ldate         = 1 << iota     // the date: 2009/01/23
	Ltime                         // the time: 01:23:23
	Lmicroseconds                 // microsecond resolution: 01:23:23.123123.  assumes Ltime.
	Llongfile                     // full file name and line number: /a/b/c/d.go:23
	Lshortfile                    // final file name element and line number: d.go:23. overrides Llongfile
	LstdFlags     = Ldate | Ltime // initial values for the standard logger

)

const (
	LEVEL_DEBUG = iota
	LEVEL_INFO
	LEVEL_WARN
	LEVEL_ERROR
	LEVEL_FATAL
)

var levelString = [...]string{
	"[DEBUG]",
	"[INFO]",
	"[WARN]",
	"[ERROR]",
	"[FATAL]",
}

// A Logger represents an active logging object that generates lines of
// output to an io.Writer.  Each logging operation makes a single call to
// the Writer's Write method.  A Logger can be used simultaneously from
// multiple goroutines; it guarantees to serialize access to the Writer.
type Logger struct {
	mu    sync.Mutex // ensures atomic writes; protects the following fields
	flag  int        // properties
	out   io.Writer  // destination for output
	buf   []byte     // for accumulating text to write
	level int
	name  string
}

// New creates a new Logger.   The out variable sets the
// destination to which log data will be written.
// The prefix appears at the beginning of each generated log line.
// The flag argument defines the logging properties.

func New(name string) *Logger {
	l := &Logger{out: os.Stderr, flag: LstdFlags, level: LEVEL_DEBUG, name: name}

	add(l)

	return l
}

// Cheap integer to fixed-width decimal ASCII.  Give a negative width to avoid zero-padding.
// Knows the buffer has capacity.
func itoa(buf *[]byte, i int, wid int) {
	var u uint = uint(i)
	if u == 0 && wid <= 1 {
		*buf = append(*buf, '0')
		return
	}

	// Assemble decimal in reverse order.
	var b [32]byte
	bp := len(b)
	for ; u > 0 || wid > 0; u /= 10 {
		bp--
		wid--
		b[bp] = byte(u%10) + '0'
	}
	*buf = append(*buf, b[bp:]...)
}

func (self *Logger) formatHeader(buf *[]byte, t time.Time, file string, line int, prefix string) {
	*buf = append(*buf, prefix...)
	*buf = append(*buf, ' ')
	if self.flag&(Ldate|Ltime|Lmicroseconds) != 0 {
		if self.flag&Ldate != 0 {
			year, month, day := t.Date()
			itoa(buf, year, 4)
			*buf = append(*buf, '/')
			itoa(buf, int(month), 2)
			*buf = append(*buf, '/')
			itoa(buf, day, 2)
			*buf = append(*buf, ' ')
		}
		if self.flag&(Ltime|Lmicroseconds) != 0 {
			hour, min, sec := t.Clock()
			itoa(buf, hour, 2)
			*buf = append(*buf, ':')
			itoa(buf, min, 2)
			*buf = append(*buf, ':')
			itoa(buf, sec, 2)
			if self.flag&Lmicroseconds != 0 {
				*buf = append(*buf, '.')
				itoa(buf, t.Nanosecond()/1e3, 6)
			}
			*buf = append(*buf, ' ')
		}
	}
	if self.flag&(Lshortfile|Llongfile) != 0 {
		if self.flag&Lshortfile != 0 {
			short := file
			for i := len(file) - 1; i > 0; i-- {
				if file[i] == '/' {
					short = file[i+1:]
					break
				}
			}
			file = short
		}
		*buf = append(*buf, file...)
		*buf = append(*buf, ':')
		itoa(buf, line, -1)
		*buf = append(*buf, ": "...)
	}
}

// Output writes the output for a logging event.  The string s contains
// the text to print after the prefix specified by the flags of the
// Logger.  A newline is appended if the last character of s is not
// already a newline.  Calldepth is used to recover the PC and is
// provided for generality, although at the moment on all pre-defined
// paths it will be 2.
func (self *Logger) Output(calldepth int, prefix string, s string) error {
	now := time.Now() // get this early.
	var file string
	var line int
	self.mu.Lock()
	defer self.mu.Unlock()
	if self.flag&(Lshortfile|Llongfile) != 0 {
		// release lock while getting caller info - it's expensive.
		self.mu.Unlock()
		var ok bool
		_, file, line, ok = runtime.Caller(calldepth)
		if !ok {
			file = "???"
			line = 0
		}
		self.mu.Lock()
	}
	self.buf = self.buf[:0]
	self.formatHeader(&self.buf, now, file, line, prefix)
	self.buf = append(self.buf, s...)
	if len(s) > 0 && s[len(s)-1] != '\n' {
		self.buf = append(self.buf, '\n')
	}
	_, err := self.out.Write(self.buf)
	return err
}

func (self *Logger) log(level int, format string, v ...interface{}) {

	if level < self.level {
		return
	}

	prefix := fmt.Sprintf("%s %s", levelString[level], self.name)

	if format == "" {
		self.Output(3, prefix, fmt.Sprintln(v...))
	} else {
		self.Output(3, prefix, fmt.Sprintf(format, v...))
	}

}

func (self *Logger) Debugf(format string, v ...interface{}) {

	self.log(LEVEL_DEBUG, format, v...)
}

func (self *Logger) Debugln(v ...interface{}) {
	self.log(LEVEL_DEBUG, "", v...)
}

func (self *Logger) Infof(format string, v ...interface{}) {

	self.log(LEVEL_INFO, format, v...)
}

func (self *Logger) Infoln(v ...interface{}) {
	self.log(LEVEL_INFO, "", v...)
}

func (self *Logger) Warnf(format string, v ...interface{}) {

	self.log(LEVEL_WARN, format, v...)
}

func (self *Logger) Warnln(v ...interface{}) {
	self.log(LEVEL_WARN, "", v...)
}

func (self *Logger) Errorf(format string, v ...interface{}) {

	self.log(LEVEL_ERROR, format, v...)
}

func (self *Logger) Errorln(v ...interface{}) {
	self.log(LEVEL_ERROR, "", v...)
}

func (self *Logger) Fatalf(format string, v ...interface{}) {

	self.log(LEVEL_FATAL, format, v...)
}

func (self *Logger) Fatalln(v ...interface{}) {
	self.log(LEVEL_FATAL, "", v...)
}
