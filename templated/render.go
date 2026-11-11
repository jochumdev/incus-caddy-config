package templated

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

func funcs() template.FuncMap {
	return template.FuncMap{
		"json": func(v any) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}

			return string(b), nil
		},
		"join":  strings.Join,
		"split": strings.Split,
	}
}

// render executes the target's template with data, extra funcs taking precedence over the generic ones.
func render(t Target, data Data, extra template.FuncMap) ([]byte, error) {
	// A label or flag an instance does not carry renders empty; the default prints "<no value>" into the file.
	tmpl, err := template.New(t.Template).Option("missingkey=zero").Funcs(funcs()).Funcs(extra).Parse(t.Text)
	if err != nil {
		return nil, fmt.Errorf("parsing template %q: %w", t.Template, err)
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, data)
	if err != nil {
		return nil, fmt.Errorf("rendering template %q: %w", t.Template, err)
	}

	return buf.Bytes(), nil
}
