package action

import (
	"fmt"
	"github.com/leochen2038/goplay/reconst/env"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

var registerCode string
var packages = map[string]bool{}

func ReconstAction() (err error) {
	actions, err := getActions(env.ProjectPath + "/assets/action")

	registerCode = "func init() {\n"
	for _, action := range actions {
		registerCode += "\tplay.RegisterAction(\"" + action.name + "\", " + "func()interface{}{return "
		genNextProcessorCode(action.handlerList, &action)
		registerCode = registerCode[:len(registerCode)-1] + "})\n"
	}
	registerCode += "}"
	updateRegister(env.ProjectPath, env.FrameworkName)
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
		packages[strings.ReplaceAll(proc.name[:strings.LastIndex(proc.name, ".")], ".", "/")] = true
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
	for k, _ := range packages {
		src += fmt.Sprintf("\t\"%s/processor/%s\"\n", module, k)
	}
	src += "\"unsafe\"\n"
	src += ")\n\n"

	src += registerCode
	path := fmt.Sprintf("%s/register.go", project)
	if err = ioutil.WriteFile(path, []byte(src), 0644); err != nil {
		return
	}

	exec.Command("gofmt", "-w", path).Run()
	return nil
}
