package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"gopkg.in/fsnotify.v1"

	"git.zxq.co/ripple/hanayo/apiclient"
	"git.zxq.co/ripple/rippleapi/common"
	"github.com/gin-gonic/contrib/sessions"
	"github.com/gin-gonic/gin"
)

var templates = make(map[string]*template.Template)
var baseTemplates = [...]string{
	"templates/base.html",
	"templates/navbar.html",
}
var funcMap = template.FuncMap{
	"html": func(value interface{}) template.HTML {
		return template.HTML(fmt.Sprint(value))
	},
	"avatars": func() string {
		return config.AvatarURL
	},
	"navbarItem": func(currentPath, name, path string) template.HTML {
		var act string
		if path == currentPath {
			act = "active "
		}
		return template.HTML(fmt.Sprintf(`<a class="%sitem" href="%s">%s</a>`, act, path, name))
	},
	"curryear": func() string {
		return strconv.Itoa(time.Now().Year())
	},
	"hasAdmin": func(privs int64) bool {
		return privs&common.AdminPrivilegeAccessRAP > 0
	},
	"isRAP": func(p string) bool {
		parts := strings.Split(p, "/")
		return len(parts) > 1 && parts[1] == "admin"
	},
	"favMode": func(favMode, current float64) string {
		if favMode == current {
			return "active "
		}
		return ""
	},
	"slice": func(els ...interface{}) []interface{} {
		return els
	},
	"int": func(f float64) int {
		return int(f)
	},
	"parseUserpage": func(s string) template.HTML {
		return template.HTML(compileBBCode(s))
	},
	"get": apiclient.Get,
}

func loadTemplates(subdir string) {
	ts, err := ioutil.ReadDir("templates" + subdir)
	if err != nil {
		panic(err)
	}

	for _, i := range ts {
		// if it's a directory, load recursively
		if i.IsDir() && i.Name() != ".." && i.Name() != "." {
			loadTemplates(subdir + "/" + i.Name())
			continue
		}

		// do not compile base templates on their own
		var comp bool
		for _, j := range baseTemplates {
			if "templates"+subdir+"/"+i.Name() == j {
				comp = true
				break
			}
		}
		if comp {
			continue
		}

		var inName string
		if subdir != "" && subdir[0] == '/' {
			inName = subdir[1:] + "/"
		}

		// add new template to template slice
		templates[inName+i.Name()] = template.Must(template.New(i.Name()).Funcs(funcMap).ParseFiles(
			append([]string{"templates" + subdir + "/" + i.Name()}, baseTemplates[:]...)...,
		))
	}
}

func resp(c *gin.Context, statusCode int, tpl string, data interface{}) {
	if c == nil {
		return
	}
	t := templates[tpl]
	if t == nil {
		c.String(500, "Template not found! Please tell this to a dev!")
		return
	}
	if corrected, ok := data.(page); ok {
		corrected.SetMessages(getMessages(c))
		corrected.SetPath(c.Request.URL.Path)
		corrected.SetContext(c.MustGet("context").(context))
	}
	sess := c.MustGet("session").(sessions.Session)
	sess.Save()
	buf := &bytes.Buffer{}
	err := t.ExecuteTemplate(buf, "base", data)
	if err != nil {
		c.Writer.WriteString("oooops! A monkey stumbled upon a banana while trying to process your request. " +
			"This doesn't make much sense, but in a few words: we fucked up something while processing your " +
			"request. We are sorry for this, but don't worry: we have been notified and are on it!")
		c.Error(err)
		return
	}
	c.Status(statusCode)
	_, err = io.Copy(c.Writer, buf)
	if err != nil {
		c.Writer.WriteString("We don't know what's happening now.")
		c.Error(err)
		return
	}
}

type baseTemplateData struct {
	TitleBar     string
	HeadingTitle string
	Scripts      []string
	KyutGrill    string
	Context      context
	Path         string
	Messages     []message
	FormData     map[string]string
}

func (b *baseTemplateData) SetMessages(m []message) {
	b.Messages = append(b.Messages, m...)
}
func (b *baseTemplateData) SetPath(path string) {
	b.Path = path
}
func (b *baseTemplateData) SetContext(c context) {
	b.Context = c
}

type page interface {
	SetMessages([]message)
	SetPath(string)
	SetContext(context)
}

func reloader() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	err = w.Add("templates")
	if err != nil {
		w.Close()
		return err
	}
	go func() {
		for {
			select {
			case ev := <-w.Events:
				if ev.Op&fsnotify.Write == fsnotify.Write {
					fmt.Println("Change detected! Refreshing templates")
					loadTemplates("")
				}
			case err := <-w.Errors:
				fmt.Println(err)
			}
		}
	}()
	return nil
}
