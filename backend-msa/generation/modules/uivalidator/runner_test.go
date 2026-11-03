package uivalidator

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRunnerExecutesInstallBuildAndBrowserValidation(t *testing.T) {
	commands := []string{}
	runner := NewWithCommandRunner(func(_ context.Context, dir, name string, args ...string) ([]byte, error) {
		if _, err := os.Stat(dir + "/package.json"); err != nil {
			t.Fatalf("workspace not scaffolded: %v", err)
		}
		commands = append(commands, name+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	})
	results := runner.Validate(context.Background(), validDesign())
	if len(results) != 5 {
		t.Fatalf("validations = %#v", results)
	}
	for _, result := range results {
		if result.Status != "passed" {
			t.Fatalf("validation failed: %#v", result)
		}
	}
	want := []string{"npm install --ignore-scripts --no-audit --no-fund", "npx playwright install chromium", "npm run build", "npm run test:ui"}
	if strings.Join(commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestBuildFailureCannotProduceExecutableValidation(t *testing.T) {
	runner := NewWithCommandRunner(func(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
		if name == "npm" && len(args) > 1 && args[0] == "run" && args[1] == "build" {
			return []byte("typescript error"), errors.New("exit 2")
		}
		return nil, nil
	})
	for _, result := range runner.Validate(context.Background(), validDesign()) {
		if result.Status == "passed" {
			t.Fatalf("failure was accepted: %#v", result)
		}
	}
}

func TestRealGeneratedUIValidation(t *testing.T) {
	if os.Getenv("NOPLANNER_UI_VALIDATOR_INTEGRATION") != "1" {
		t.Skip("set NOPLANNER_UI_VALIDATOR_INTEGRATION=1 to run npm and Chromium")
	}
	for _, result := range New().Validate(context.Background(), validDesign()) {
		if result.Status != "passed" {
			t.Fatalf("real validation failed: %#v", result)
		}
	}
}

func validDesign() Product {
	return Product{ScreenIDs: []string{"screen-REQ-1"}, Files: []File{{Path: "src/generated/screen-REQ-1.tsx", ScreenIDs: []string{"screen-REQ-1"}, Content: `import {useState} from 'react'; import {Button} from '@/components/ui/button'; export function screen_REQ_1({state='normal'}:{state?:'normal'|'loading'|'empty'|'error'|'forbidden'}){const[done,setDone]=useState(false);return <main data-state={state} className="md:p-8" aria-labelledby="title"><h1 id="title">Title</h1>{state==='normal'?<><p role="status">{done?'완료되었습니다.':'실행할 수 있습니다.'}</p><Button onClick={()=>setDone(true)}>계속</Button></>:<p role={state==='error'?'alert':'status'}>{state}</p>}</main>}`}}}
}
