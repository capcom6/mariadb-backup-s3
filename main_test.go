package main

import (
	"context"
	"testing"
)

func Test_isInterrupted(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Context not done",
			args: args{ctx: context.Background()},
			want: false,
		},
		{
			name: "Context done",
			args: args{ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}()},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInterrupted(tt.args.ctx); got != tt.want {
				t.Errorf("isIterrupted() = %v, want %v", got, tt.want)
			}
		})
	}
}
