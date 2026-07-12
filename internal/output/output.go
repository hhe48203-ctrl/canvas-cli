package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

type Envelope struct {
	OK      bool   `json:"ok" yaml:"ok"`
	Data    any    `json:"data,omitempty" yaml:"data,omitempty"`
	Error   any    `json:"error,omitempty" yaml:"error,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

func Success(data any) Envelope { return Envelope{OK: true, Data: data} }

func Failure(err error) Envelope {
	return Envelope{OK: false, Error: map[string]any{"message": err.Error()}}
}

func Print(value any, format string) error {
	return PrintTo(os.Stdout, value, format)
}

func PrintTo(w io.Writer, value any, format string) error {
	switch format {
	case "json":
		return json.NewEncoder(w).Encode(value)
	case "yaml":
		data, err := yaml.Marshal(value)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	case "table":
		return printTable(w, value)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func printTable(w io.Writer, value any) error {
	if envelope, ok := value.(Envelope); ok {
		if envelope.OK {
			value = envelope.Data
		} else {
			_, err := fmt.Fprintln(w, envelope.Error)
			return err
		}
	}
	if items, ok := asItems(value); ok && len(items) > 0 {
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tNAME\tSTATUS")
		for _, item := range items {
			name := first(item, "name", "summary", "title")
			status := first(item, "workflow_state", "status", "method")
			fmt.Fprintf(tw, "%v\t%v\t%v\n", item["id"], name, status)
		}
		return tw.Flush()
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func asItems(value any) ([]map[string]any, bool) {
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Slice {
		return nil, false
	}
	items := make([]map[string]any, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		item, ok := v.Index(i).Interface().(map[string]any)
		if !ok {
			return nil, false
		}
		items = append(items, item)
	}
	return items, true
}

func first(item map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			return value
		}
	}
	return ""
}
