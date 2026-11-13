// Copyright 2016 fatedier, fatedier@gmail.com
// Copyright 2026 eoasmxd (Modified for L4Tunnel)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"fmt"
	stdlog "log"
	"os"
	"sync"
	"sync/atomic"
)

const (
	levelError int32 = 1
	levelWarn  int32 = 2
	levelInfo  int32 = 3
	levelDebug int32 = 4
	levelTrace int32 = 5
)

var (
	stdLogger = stdlog.New(os.Stdout, "", stdlog.LstdFlags)
	curLevel  = levelInfo
)

type ctxKey int

const logCtxKey ctxKey = 0

// InitLog 初始化日志输出级别，日志统一输出到标准输出。
func InitLog(logLevel string) {
	SetLogLevel(logLevel)
}

// SetLogLevel 设置日志输出级别。
func SetLogLevel(logLevel string) {
	var level int32
	switch logLevel {
	case "error":
		level = levelError
	case "warn":
		level = levelWarn
	case "info":
		level = levelInfo
	case "debug":
		level = levelDebug
	case "trace":
		level = levelTrace
	default:
		level = levelInfo
	}
	atomic.StoreInt32(&curLevel, level)
}

func output(level int32, tag string, format string, v ...interface{}) {
	if atomic.LoadInt32(&curLevel) < level {
		return
	}
	var msg string
	if len(v) == 0 {
		msg = format
	} else {
		msg = fmt.Sprintf(format, v...)
	}
	stdLogger.Output(3, tag+" "+msg)
}

func Error(format string, v ...interface{}) { output(levelError, "[E]", format, v...) }
func Warn(format string, v ...interface{})  { output(levelWarn, "[W]", format, v...) }
func Info(format string, v ...interface{})  { output(levelInfo, "[I]", format, v...) }
func Debug(format string, v ...interface{}) { output(levelDebug, "[D]", format, v...) }
func Trace(format string, v ...interface{}) { output(levelTrace, "[T]", format, v...) }

// Logger 支持上下文前缀拼接的轻量线程安全日志器。
type Logger struct {
	mu           sync.RWMutex
	prefixes     []string
	prefixString string
}

func NewLogger() *Logger {
	return &Logger{
		prefixes: make([]string, 0),
	}
}

func (l *Logger) ResetPrefixes() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefixes = make([]string, 0)
	l.prefixString = ""
}

func (l *Logger) AppendPrefix(prefix string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefixes = append(l.prefixes, prefix)
	l.prefixString += "[" + prefix + "] "
	return l
}

func (l *Logger) Spawn() *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()
	nl := NewLogger()
	for _, v := range l.prefixes {
		nl.AppendPrefix(v)
	}
	return nl
}

func (l *Logger) Error(format string, v ...interface{}) {
	l.mu.RLock()
	p := l.prefixString
	l.mu.RUnlock()
	output(levelError, "[E]", p+format, v...)
}

func (l *Logger) Warn(format string, v ...interface{}) {
	l.mu.RLock()
	p := l.prefixString
	l.mu.RUnlock()
	output(levelWarn, "[W]", p+format, v...)
}

func (l *Logger) Info(format string, v ...interface{}) {
	l.mu.RLock()
	p := l.prefixString
	l.mu.RUnlock()
	output(levelInfo, "[I]", p+format, v...)
}

func (l *Logger) Debug(format string, v ...interface{}) {
	l.mu.RLock()
	p := l.prefixString
	l.mu.RUnlock()
	output(levelDebug, "[D]", p+format, v...)
}

func (l *Logger) Trace(format string, v ...interface{}) {
	l.mu.RLock()
	p := l.prefixString
	l.mu.RUnlock()
	output(levelTrace, "[T]", p+format, v...)
}

func NewLogContext(ctx context.Context, xl *Logger) context.Context {
	return context.WithValue(ctx, logCtxKey, xl)
}

func LogFromContextSafe(ctx context.Context) *Logger {
	if xl, ok := ctx.Value(logCtxKey).(*Logger); ok {
		return xl
	}
	return NewLogger()
}

