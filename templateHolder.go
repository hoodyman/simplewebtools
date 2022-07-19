package simplewebtools

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"text/template"
)

type TemplateHolder struct {
	m         sync.Mutex
	templates *template.Template
}

func (t *TemplateHolder) Apply(wr io.Writer, name string, data any) error {
	t.m.Lock()
	var err error
	if t.templates != nil {
		err = t.templates.ExecuteTemplate(wr, name, data)
	} else {
		err = errors.New("template holder: templates not loaded")
	}
	t.m.Unlock()
	return err
}

func (t *TemplateHolder) LoadTemplates(path string) error {
	tmplts := []string{}
	var err error

	err = filepath.Walk(path,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				tmplts = append(tmplts, path)
			}
			return nil
		})
	if err != nil {
		return err
	}

	t.m.Lock()
	t.templates, err = template.ParseFiles(tmplts...)
	t.m.Unlock()
	return err
}
