package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

var (
	// TaskFilePath is the default Taskfile
	TaskFilePath = "Taskfile"
	// ShExists is true if Bash was found
	ShExists bool
	// ShPath constains the Bash path if found
	ShPath string

	// Tasks constains the tasks parsed from Taskfile
	Tasks = make(map[string]*Task)

	runTasks = make(map[string]bool)
)

func init() {
	var err error
	ShPath, err = exec.LookPath("sh")
	if err != nil {
		return
	}
	ShExists = true
}

// Task represents a task
type Task struct {
	Cmds      []string
	Deps      []string
	Sources   []string
	Generates []string
	Dir       string
	Variables map[string]string
	Set       string
}

type taskNotFoundError struct {
	taskName string
}

func (err *taskNotFoundError) Error() string {
	return fmt.Sprintf(`Task "%s" not found`, err.taskName)
}

type taskRunError struct {
	taskName string
	err      error
}

func (err *taskRunError) Error() string {
	return fmt.Sprintf(`Failed to run task "%s": %v`, err.taskName, err.err)
}

// Run runs Task
func Run() {
	log.SetFlags(0)

	args := os.Args[1:]
	if len(args) == 0 {
		log.Fatal("No argument given")
	}

	var err error
	Tasks, err = readTaskfile()
	if err != nil {
		log.Fatal(err)
	}

	for _, a := range args {
		if err = RunTask(a); err != nil {
			log.Fatal(err)
		}
	}
}

// RunTask runs a task by its name
func RunTask(name string) error {
	if _, found := runTasks[name]; found {
		return &taskRunError{taskName: name, err: fmt.Errorf("Cyclic dependency detected")}
	}
	runTasks[name] = true

	t, ok := Tasks[name]
	if !ok {
		return &taskNotFoundError{name}
	}

	if isTaskUpToDate(t) {
		log.Printf(`Task "%s" is up to date`, name)
		return nil
	}
	vars, err := t.handleVariables()
	if err != nil {
		return &taskRunError{name, err}
	}
	for _, d := range t.Deps {
		if err := RunTask(ReplaceVariables(d, vars)); err != nil {
			return err
		}
	}
	for _, c := range t.Cmds {
		// read in a each time, as a command could change a variable or it has been changed by a dependency
		vars, err = t.handleVariables()
		if err != nil {
			return &taskRunError{name, err}
		}
		var (
			output string
			err    error
		)
		if output, err = runCommand(ReplaceVariables(c, vars), ReplaceVariables(t.Dir, vars)); err != nil {
			return &taskRunError{name, err}
		}
		fmt.Println(output)
		if t.Set != "" {
			os.Setenv(t.Set, output)
		}
	}
	return nil
}

func isTaskUpToDate(t *Task) bool {
	if len(t.Sources) == 0 || len(t.Generates) == 0 {
		return false
	}

	sourcesMaxTime, err := getPatternsMaxTime(t.Sources)
	if err != nil || sourcesMaxTime.IsZero() {
		return false
	}

	generatesMinTime, err := getPatternsMinTime(t.Generates)
	if err != nil || generatesMinTime.IsZero() {
		return false
	}

	return generatesMinTime.After(sourcesMaxTime)
}

func runCommand(c, path string) (string, error) {
	var (
		cmd *exec.Cmd
		b   []byte
		err error
	)
	if ShExists {
		cmd = exec.Command(ShPath, "-c", c)
	} else {
		cmd = exec.Command("cmd", "/C", c)
	}
	if path != "" {
		cmd.Dir = path
	}
	cmd.Stderr = os.Stderr
	if b, err = cmd.Output(); err != nil {
		return "", err
	}
	return string(b), nil
}

func readTaskfile() (tasks map[string]*Task, err error) {
	if b, err := ioutil.ReadFile(TaskFilePath + ".yml"); err == nil {
		return tasks, yaml.Unmarshal(b, &tasks)
	}
	if b, err := ioutil.ReadFile(TaskFilePath + ".json"); err == nil {
		return tasks, json.Unmarshal(b, &tasks)
	}
	if b, err := ioutil.ReadFile(TaskFilePath + ".toml"); err == nil {
		return tasks, toml.Unmarshal(b, &tasks)
	}
	return nil, ErrNoTaskFile
}

// ErrNoTaskFile is returns when the program can not find a proper TaskFile
var ErrNoTaskFile = errors.New("no task file found (is it named '" + TaskFilePath + "'?)")
