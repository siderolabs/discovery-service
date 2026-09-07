// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestResolveLogLevel(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		in      string
		want    zapcore.Level
		wantErr bool
	}{
		{name: "empty defaults to defaultLogLevel", in: "", want: defaultLogLevel},
		{name: "debug", in: "debug", want: zapcore.DebugLevel},
		{name: "info", in: "info", want: zapcore.InfoLevel},
		{name: "warn", in: "warn", want: zapcore.WarnLevel},
		{name: "error", in: "error", want: zapcore.ErrorLevel},
		{name: "dpanic", in: "dpanic", want: zapcore.DPanicLevel},
		{name: "panic", in: "panic", want: zapcore.PanicLevel},
		{name: "fatal", in: "fatal", want: zapcore.FatalLevel},
		{name: "uppercase is accepted", in: "DEBUG", want: zapcore.DebugLevel},
		{name: "invalid identifier errors", in: "bogus", wantErr: true},
		{name: "trailing whitespace errors", in: "info ", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveLogLevel(tt.in)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultLogLevelIsInfo(t *testing.T) {
	t.Parallel()

	// the flag's usage text and help output hardcode "info" as the advertised default,
	// so pin defaultLogLevel to it to catch accidental drift.
	assert.Equal(t, zapcore.InfoLevel, defaultLogLevel)
}
