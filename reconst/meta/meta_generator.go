package meta

import (
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/leochen2038/goplay/reconst/env"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

type Meta struct {
	XMLName  xml.Name     `xml:"meta"`
	Module   string       `xml:"module,attr"`
	Name     string       `xml:"name,attr"`
	Tag      string       `xml:"tag,attr"`
	Key      MetaField    `xml:"key"`
	Fields   MetaFields   `xml:"fields"`
	Strategy MetaStrategy `xml:"strategy"`
}

type MetaFields struct {
	List []MetaField `xml:"field"`
}

type MetaField struct {
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Note    string `xml:"note,attr"`
	Default string `xml:"default,attr"`
}

type MetaStrategy struct {
	Storage MetaStorage `xml:"storage"`
}

type MetaStorage struct {
	Type     string `xml:"type,attr"`
	Drive    string `xml:"drive,attr"`
	Database string `xml:"database,attr"`
	Table    string `xml:"table,attr"`
	Router   string `xml:"router,attr"`
}

func MetaGenerator() error {
	return filepath.Walk(env.ProjectPath+"/assets/meta", func(filename string, fi os.FileInfo, err error) error {
		var data []byte
		var meta Meta
		if !fi.IsDir() && strings.HasSuffix(filename, ".xml") {
			if data, err = ioutil.ReadFile(filename); err != nil {
				return err
			}
			if err = xml.Unmarshal(data, &meta); err != nil {
				return errors.New("check: " + filename + " failure:" + err.Error())
			}
			if err = writeMeta(meta); err != nil {
				return errors.New("check: " + filename + " failure: " + err.Error())
			}
			fmt.Println("check:", filename, "success")
		}
		return nil
	})
}

func formatLowerName(name string) string {
	return strings.ToLower(strings.Join(strings.Split(name, "_"), ""))
}

func formatUcfirstName(name string) string {
	var split []string
	for _, v := range strings.Split(name, "_") {
		split = append(split, ucfirst(v))
	}
	return strings.Join(split, "")
}

func generateCode(meta Meta) string {
	whereOr := map[string]string{"Where": "true", "Or": "false"}
	con1List := [...]string{"Equal", "NotEqual", "Less", "Greater", "Like"}
	con2List := [...]string{"Between"}
	conslice := [...]string{"In"}

	funcName := formatUcfirstName(meta.Module) + formatUcfirstName(meta.Name)
	src := "package db\n"
	if meta.Strategy.Storage.Type == "mongodb" {
		src += fmt.Sprintf(`
import (
	"%s"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"%s/database/%s"
	"time"
)
`, env.FrameworkName, env.FrameworkName, meta.Strategy.Storage.Drive)
	} else {
		src += fmt.Sprintf(`
import (
	"%s"
	"%s/database/%s"
)
`, env.FrameworkName, env.FrameworkName, meta.Strategy.Storage.Drive)
	}

	src += genSubObject(meta, funcName)
	src += fmt.Sprintf("\ntype Meta%s struct {\n", funcName)
	src += "\t" + formatUcfirstName(meta.Key.Name) + " "
	if meta.Strategy.Storage.Type == "mongodb" {
		src += "primitive.ObjectID\t `bson:\"" + meta.Key.Name + "\"`\n"
	} else {
		if meta.Key.Type == "auto" {
			src += "int\t `db:\"" + meta.Key.Name + "\"`\n"
		} else {
			src += meta.Key.Type + "\t `db:\"" + meta.Key.Name + "\"`\n"
		}
	}
	for _, vb := range meta.Fields.List {
		src += "\t" + ucfirst(vb.Name) + " " + getGolangType(vb.Type)
		if meta.Strategy.Storage.Type == "mongodb" {
			src += "\t `bson:\"" + vb.Name + "\"`\n"
		} else {
			src += "\t `db:\"" + vb.Name + "\"`\n"
		}
	}
	src += "}\n"

	for _, vb := range meta.Fields.List {
		src += fmt.Sprintf(`func (meta *Meta%s)Set%s(val %s) *Meta%s {
	meta.%s = val
	return meta
}
`, funcName, ucfirst(vb.Name), getGolangType(vb.Type), funcName, ucfirst(vb.Name))
	}
	src += "\n"

	src += fmt.Sprintf(`
type query%s struct {
	query play.Query
}
`, funcName)

	var initFields string
	for _, field := range meta.Fields.List {
		initFields += fmt.Sprintf(`"%s":true,`, field.Name)
	}
	initFields += fmt.Sprintf(`"%s":true`, meta.Key.Name)

	src += fmt.Sprintf(`
func %s() *query%s {
	obj := &query%s{}
	obj.query.Module = "%s"
	obj.query.Name = "%s"
	obj.query.DBName = "%s"
	obj.query.Table = "%s"
	obj.query.Router = "%s"
	obj.query.Sets = map[string][]interface{}{}
	obj.query.Fields = map[string]bool{%s}
	return obj
}
`, funcName, funcName, funcName, meta.Module, meta.Name, meta.Strategy.Storage.Database, meta.Strategy.Storage.Table, meta.Strategy.Storage.Router, initFields)

	for _, cond := range con1List {
		// generate key
		for where, wherebool := range whereOr {
			src += fmt.Sprintf(`
func (q *query%s)%s%s%s(val interface{}) *query%s {
	q.query.Conditions = append(q.query.Conditions, play.Condition{AndOr:%s, Field:"%s", Con:"%s", Val:val})
	return q
}
`, funcName, where, formatUcfirstName(meta.Key.Name), cond, funcName, wherebool, meta.Key.Name, cond)
		}

		// generate fields
		for _, vb := range meta.Fields.List {
			for where, wherebool := range whereOr {
				src += fmt.Sprintf(`
func (q *query%s)%s%s%s(val interface{}) *query%s {
	q.query.Conditions = append(q.query.Conditions, play.Condition{AndOr:%s, Field:"%s", Con:"%s", Val:val})
	return q
}
`, funcName, where, ucfirst(vb.Name), cond, funcName, wherebool, vb.Name, cond)
			}
		}
	}

	for _, cond := range con2List {
		// generate key
		for where, wherebool := range whereOr {
			src += fmt.Sprintf(`
func (q *query%s)%s%s%s(v1 interface{}, v2 interface{}) *query%s {
	q.query.Conditions = append(q.query.Conditions, play.Condition{AndOr:%s, Field:"%s", Con:"%s", Val:[2]interface{}{v1, v2}})
	return q
}
`, funcName, where, formatUcfirstName(meta.Key.Name), cond, funcName, wherebool, meta.Key.Name, cond)
		}

		// generate fields
		for _, vb := range meta.Fields.List {
			for where, wherebool := range whereOr {
				src += fmt.Sprintf(`
func (q *query%s)%s%s%s(v1 interface{}, v2 interface{}) *query%s {
	q.query.Conditions = append(q.query.Conditions, play.Condition{AndOr:%s, Field:"%s", Con:"%s", Val:[2]interface{}{v1, v2}})
	return q
}
`, funcName, where, ucfirst(vb.Name), cond, funcName, wherebool, vb.Name, cond)
			}
		}
	}

	for _, cond := range conslice {
		// generate key
		for where, wherebool := range whereOr {
			src += fmt.Sprintf(`
func (q *query%s)%s%s%s(s []interface{}) *query%s {
	q.query.Conditions = append(q.query.Conditions, play.Condition{AndOr:%s, Field:"%s", Con:"%s", Val:s})
	return q
}
`, funcName, where, formatUcfirstName(meta.Key.Name), cond, funcName, wherebool, meta.Key.Name, cond)
		}

		// generate fields
		for _, vb := range meta.Fields.List {
			for where, wherebool := range whereOr {
				src += fmt.Sprintf(`
func (q *query%s)%s%s%s(s []%s) *query%s {
	q.query.Conditions = append(q.query.Conditions, play.Condition{AndOr:%s, Field:"%s", Con:"%s", Val:s})
	return q
}
`, funcName, where, ucfirst(vb.Name), cond, getGolangType(vb.Type), funcName, wherebool, vb.Name, cond)
			}
		}

	}

	src += fmt.Sprintf(`
func (q *query%s)OrderBy(key, val string) *query%s {
	q.query.Order = append(q.query.Order, [2]string{key, val})
	return q
}
`, funcName, funcName)

	src += fmt.Sprintf(`
func (q *query%s)Count() (int64, error) {
	return %s.Count(&q.query)
}
`, funcName, meta.Strategy.Storage.Drive)

	src += fmt.Sprintf(`
func (q *query%s)Delete() (int64, error) {
	return %s.Delete(&q.query)
}
`, funcName, meta.Strategy.Storage.Drive)

	src += fmt.Sprintf(`
func (q *query%s)Limit(start int64, count int64) *query%s {
	q.query.Limit[0] = start
	q.query.Limit[1] = count
	return q
}
`, funcName, funcName)

	if meta.Strategy.Storage.Type == "mongodb" {
		src += fmt.Sprintf(`
func (q *query%s)GetOne() (*Meta%s, error) {
	meta := &Meta%s{}
	if err := %s.GetOne(meta, &q.query); err != nil {
		return nil, err 
	}
	return meta, nil
}
`, funcName, funcName, funcName, meta.Strategy.Storage.Drive)
	} else {
		src += fmt.Sprintf(`
func (q *query%s)GetOne() (*Meta%s, error) {
	meta := &Meta%s{}
	if err := %s.GetOne(meta, &q.query); err != nil {
		return nil, err 
	}
	return meta, nil
}
`, funcName, funcName, funcName, meta.Strategy.Storage.Drive)
	}

	src += fmt.Sprintf(`
func (q *query%s)GetList() ([]Meta%s, error) {
	list := []Meta%s{}
	err := %s.GetList(&list, &q.query)
	return list, err
}
`, funcName, funcName, funcName, meta.Strategy.Storage.Drive)

	if meta.Strategy.Storage.Type == "mongodb" {
		src += fmt.Sprintf(`
func (q *query%s)Save(meta *Meta%s) error {
	meta.Fmtime = time.Now().Unix()
	if meta.Id != primitive.NilObjectID {
		return %s.Save(meta, &meta.Id, &q.query)
	}

	meta.Fctime = time.Now().Unix()
	meta.Id = primitive.NewObjectID()
	return %s.Save(meta, nil, &q.query)
}
`, funcName, funcName, meta.Strategy.Storage.Drive, meta.Strategy.Storage.Drive)
	} else {
		src += fmt.Sprintf(`
func (q *query%s)Save(meta *Meta%s) error {
	return %s.Save(meta, &q.query)
}
`, funcName, funcName, meta.Strategy.Storage.Drive)
	}

	src += fmt.Sprintf(`
func (q *query%s)Update() (int64, error) {
	return %s.Update(&q.query)
}
`, funcName, meta.Strategy.Storage.Drive)

	src += fmt.Sprintf(`
func (q *query%s)NewMeta() *Meta%s {
	return &Meta%s{%s}
}
`, funcName, funcName, funcName, metaDefaultValue(meta.Fields.List))

	for _, field := range meta.Fields.List {
		src += fmt.Sprintf(`
func (q *query%s)Set%s(val %s, opt ...string) *query%s {
	args := make([]interface{}, 0, 2)
	if len(opt) > 0 {
		args = append(args, val, opt[0])
	} else {
		args = append(args, val)
	}
	q.query.Sets["%s"] = args
	return q
}
`, funcName, formatUcfirstName(field.Name), getGolangType(field.Type), funcName, field.Name)
	}
	return src
}

func writeMeta(meta Meta) (err error) {
	var supportDBs = []string{"mysql", "mongodb"}
	var unSupportDB = true
	for _, v := range supportDBs {
		if v == strings.ToLower(meta.Strategy.Storage.Type) {
			unSupportDB = false
			break
		}
	}
	if unSupportDB {
		return errors.New("unSupportDB " + meta.Strategy.Storage.Type)
	}

	path := env.ProjectPath + "/library/db"
	if err := os.MkdirAll(path, 0744); err != nil {
		return err
	}

	filePath := fmt.Sprintf("%s/library/db/%s_%s.go", env.ProjectPath, formatLowerName(meta.Module), formatLowerName(meta.Name))
	src := generateCode(meta)
	if err = ioutil.WriteFile(filePath, []byte(src), 0644); err != nil {
		return
	}

	exec.Command(runtime.GOROOT()+"/bin/gofmt", "-w", filePath).Run()
	return
}

func metaDefaultValue(list []MetaField) string {
	var s []string
	for _, field := range list {
		if field.Type == "string" {
			s = append(s, fmt.Sprintf(`%s:"%s"`, ucfirst(field.Name), field.Default))
		} else if field.Type == "int" {
			s = append(s, fmt.Sprintf(`%s:%s`, ucfirst(field.Name), field.Default))
		}
	}
	return strings.Join(s, ", ")
}

func genSubObject(meta Meta, funcName string) (code string) {
	for _, v := range meta.Fields.List {
		if strings.HasPrefix(v.Type, "array:{") && strings.HasSuffix(v.Type, "}") {
			keys := strings.Split(v.Type[7:len(v.Type)-1], ",")
			code = code + fmt.Sprintf("type Meta%s%s struct {\n", funcName, formatUcfirstName(v.Name))
			for _, v := range keys {
				code += "\t" + strings.ReplaceAll(strings.TrimSpace(v), ":", "\t") + "\n"
			}
			code += "}\n"
		}
	}
	return code
}

func getGolangType(t string) string {
	if strings.HasPrefix(t, "array") {
		switch t {
		case "array":
			return "[]interface{}"
		case "array:int":
			return "[]int"
		case "array:string":
			return "[]string"
		case "array:float":
			return "[]float64"
		case "array:array":
			return "[]interface{}"
		case "array:object":
			return "[]interface{}"
		}
	}
	if t == "ctime" || t == "mtime" {
		return "int64"
	}
	if t == "float" {
		return "float64"
	}

	return t
}

func ucfirst(str string) string {
	for i, v := range str {
		return string(unicode.ToUpper(v)) + str[i+1:]
	}
	return ""
}
