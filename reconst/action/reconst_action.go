package action

import (
	"fmt"
	"github.com/leochen2038/goplay/reconst/env"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var registerCode string
var packages = map[string]struct{}{}
var crontab = map[string]struct{}{}

func ReconstAction() (err error) {
	actions, err := getActions(env.ProjectPath + "/assets/action")

	registerCode = "func init() {\n"
	registerCode += genRegistCrontabCode(env.ProjectPath + "/crontab")
	for _, action := range actions {
		registerCode += "\tplay.RegisterAction(\"" + action.name + "\", " + "func()interface{}{return "
		genNextProcessorCode(action.handlerList, &action)
		registerCode = registerCode[:len(registerCode)-1] + "})\n"
	}
	registerCode += "}"
	updateRegister(env.ProjectPath, env.FrameworkName)
	return
}

func genRegistCrontabCode(path string) (registCode string) {
	reJob := regexp.MustCompile(`type (\w+) struct`)
	rePack := regexp.MustCompile(`package (\w+)`)
	filepath.Walk(path, func(filename string, info os.FileInfo, err error) error {
		if info != nil && !info.IsDir() && len(info.Name()) > 3 && filepath.Ext(info.Name()) == ".go" {
			var packageName string
			code, _ := ioutil.ReadFile(filename)
			submath := reJob.FindAllSubmatch(code, -1)
			if len(submath) > 0 {
				submath := rePack.FindSubmatch(code)
				if len(submath) > 1 {
					packageName = string(submath[1])
					crontab[filepath.Dir(filename)] = struct{}{}
				}
			}
			for _, v := range submath {
				fmt.Println("register cronJob", packageName+"."+string(v[1]))
				registCode += fmt.Sprintf("play.RegisterCronJob(\"%s.%s\", func() play.CronJob {return &%s.%s{}})\n", packageName, v[1], packageName, v[1])
			}
		}
		return nil
	})
	return
}

func genNextProcessorCode(proc *processorHandler, act *action) {
	if proc == nil {
		registerCode += "nil"
	} else {
		name := proc.name
		if err := checkProcessorFile(proc.name); err != nil {
			fmt.Println(err.Error(), "in", act.name)
			os.Exit(1)
		}
		nameSlice := strings.Split(proc.name, ".")
		packages[strings.ReplaceAll(proc.name[:strings.LastIndex(proc.name, ".")], ".", "/")] = struct{}{}
		if len(nameSlice) > 2 {
			name = strings.Join(nameSlice[len(nameSlice)-2:], ".")
		}
		registerCode += "play.NewProcessorWrap(new(" + name + "),"
		registerCode += "func(p play.Processor, ctx *play.Context) (string, error) {return play.RunProcessor(unsafe.Pointer(p.(*" + name + ")), unsafe.Sizeof(*p.(*" + name + ")),p, ctx)},"
		if proc.next == nil {
			registerCode += "nil)"
		} else {
			registerCode += "map[string]*play.ProcessorWrap{"
			for _, v := range proc.next {
				registerCode += "\"" + v.rcstring + "\":"
				genNextProcessorCode(v, act)
			}
			registerCode += "})"
		}
	}
	registerCode += ","
}

func updateRegister(project, frameworkName string) (err error) {
	var module string
	if module, err = parseModuleName(project); err != nil {
		return
	}

	src := "package main\n\nimport (\n\t\"" + frameworkName + "\"\n"
	for k, _ := range crontab {
		src += fmt.Sprintf("\t\"%s\"\n", strings.Replace(k, env.ProjectPath, module, 1))
	}
	for k, _ := range packages {
		src += fmt.Sprintf("\t\"%s/processor/%s\"\n", module, k)
	}
	if len(packages) > 0 {
		src += "\"unsafe\"\n"
	}
	src += ")\n\n"

	src += registerCode
	path := fmt.Sprintf("%s/register.go", project)
	if err = ioutil.WriteFile(path, []byte(src), 0644); err != nil {
		return
	}

	exec.Command("gofmt", "-w", path).Run()
	return nil
}
