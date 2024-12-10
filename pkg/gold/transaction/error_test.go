package transaction

import (
	"testing"

	"github.com/antlr4-go/antlr/v4"
)

func TestError_SyntaxError(t *testing.T) {
	type fields struct {
		DefaultErrorListener *antlr.DefaultErrorListener
		Errors               []CompileError
		Source               string
	}
	type args struct {
		recognizer      antlr.Recognizer
		offendingSymbol any
		line            int
		column          int
		msg             string
		e               antlr.RecognitionException
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name:   "fields emptys",
			fields: fields{},
			args:   args{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Error{
				DefaultErrorListener: tt.fields.DefaultErrorListener,
				Errors:               tt.fields.Errors,
				Source:               tt.fields.Source,
			}
			tr.SyntaxError(tt.args.recognizer, tt.args.offendingSymbol, tt.args.line, tt.args.column, tt.args.msg, tt.args.e)
		})
	}
}
