package uivalidator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const executionTimeout = 5 * time.Minute

type Runner struct {
	run  func(context.Context, string, string, ...string) ([]byte, error)
	slot chan struct{}
}

type File struct {
	Path, Content string
	ScreenIDs     []string
}

type Product struct {
	ScreenIDs []string
	Files     []File
}

type Validation struct {
	ID, Kind, Status, Command, Detail string
	ScreenIDs                         []string
}

func New() *Runner { return &Runner{run: runCommand, slot: make(chan struct{}, 1)} }

func NewWithCommandRunner(run func(context.Context, string, string, ...string) ([]byte, error)) *Runner {
	return &Runner{run: run, slot: make(chan struct{}, 1)}
}

func (r *Runner) Validate(ctx context.Context, product Product) []Validation {
	results := validationSet(product.ScreenIDs)
	select {
	case r.slot <- struct{}{}:
		defer func() { <-r.slot }()
	case <-ctx.Done():
		return failAll(results, "생성 UI 실행 검증 대기 중 요청이 취소되었습니다.")
	}
	workspace, err := os.MkdirTemp("", "noplanner-ui-validation-")
	if err != nil {
		return failAll(results, "격리 작업공간을 만들지 못했습니다.")
	}
	defer os.RemoveAll(workspace)
	if err = scaffold(workspace, product); err != nil {
		return failAll(results, err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, executionTimeout)
	defer cancel()
	if output, commandErr := r.run(ctx, workspace, "npm", "install", "--ignore-scripts", "--no-audit", "--no-fund"); commandErr != nil {
		return failAll(results, commandFailure("UI 검증 의존성을 설치하지 못했습니다.", output, commandErr))
	}
	if output, commandErr := r.run(ctx, workspace, "npx", "playwright", "install", "chromium"); commandErr != nil {
		return failAll(results, commandFailure("격리 브라우저를 준비하지 못했습니다.", output, commandErr))
	}
	if output, commandErr := r.run(ctx, workspace, "npm", "run", "build"); commandErr != nil {
		return failKind(results, "build", commandFailure("생성 UI를 빌드하지 못했습니다.", output, commandErr), true)
	}
	results = passKind(results, "build", "npm run build", "격리된 React 프로젝트의 TypeScript 및 Vite 빌드를 통과했습니다.")
	if output, commandErr := r.run(ctx, workspace, "npm", "run", "test:ui"); commandErr != nil {
		return failBrowserResults(results, commandFailure("생성 UI 브라우저 검증에 실패했습니다.", output, commandErr))
	}
	for index := range results {
		if results[index].Kind == "build" {
			continue
		}
		results[index].Status = "passed"
		results[index].Command = "playwright chromium"
		results[index].Detail = "격리된 Chromium에서 " + results[index].Kind + " 검증을 통과했습니다."
	}
	return results
}

func runCommand(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "CI=1")
	return command.CombinedOutput()
}

func validationSet(screenIDs []string) []Validation {
	results := make([]Validation, 0, 5)
	for _, kind := range []string{"build", "render", "interaction", "accessibility", "responsive"} {
		results = append(results, Validation{ID: "execution-" + kind, Kind: kind, Status: "failed", Detail: "실행 검증이 완료되지 않았습니다.", ScreenIDs: screenIDs})
	}
	return results
}

func passKind(values []Validation, kind, command, detail string) []Validation {
	for index := range values {
		if values[index].Kind == kind {
			values[index].Status, values[index].Command, values[index].Detail = "passed", command, detail
		}
	}
	return values
}

func failKind(values []Validation, kind, detail string, cascade bool) []Validation {
	for index := range values {
		if values[index].Kind == kind || cascade {
			values[index].Status = "failed"
			values[index].Detail = detail
		}
	}
	return values
}

func failAll(values []Validation, detail string) []Validation {
	return failKind(values, "", detail, true)
}

func failBrowserResults(values []Validation, detail string) []Validation {
	for index := range values {
		if values[index].Kind != "build" {
			values[index].Status = "failed"
			values[index].Detail = detail
		}
	}
	return values
}

func commandFailure(message string, output []byte, err error) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 3000 {
		text = text[len(text)-3000:]
	}
	if text == "" {
		text = err.Error()
	}
	return message + " " + text
}

var componentPattern = regexp.MustCompile(`export function ([A-Za-z_][A-Za-z0-9_]*)`)

func scaffold(root string, product Product) error {
	if len(product.Files) == 0 {
		return errors.New("검증할 생성 UI 파일이 없습니다.")
	}
	if len(product.Files) > 100 {
		return errors.New("한 번에 검증할 수 있는 생성 UI 파일 수를 초과했습니다.")
	}
	totalSize := 0
	for _, file := range product.Files {
		totalSize += len(file.Content)
	}
	if totalSize > 2*1024*1024 {
		return errors.New("생성 UI 코드가 2 MiB 검증 한도를 초과했습니다.")
	}
	files := map[string]string{
		"package.json":                 `{"private":true,"type":"module","scripts":{"build":"tsc -b && vite build","test:ui":"playwright test"},"dependencies":{"@vitejs/plugin-react":"4.3.4","@playwright/test":"1.55.0","@axe-core/playwright":"4.10.2","@types/node":"22.15.3","@types/react":"19.0.0","@types/react-dom":"19.0.0","vite":"6.4.3","typescript":"5.7.2","react":"19.0.0","react-dom":"19.0.0"},"devDependencies":{}}`,
		"index.html":                   `<!doctype html><html lang="ko"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"><title>Generated UI validation</title></head><body><div id="root"></div><script type="module" src="/src/main.tsx"></script></body></html>`,
		"tsconfig.json":                `{"compilerOptions":{"target":"ES2022","useDefineForClassFields":true,"lib":["ES2022","DOM","DOM.Iterable"],"allowJs":false,"skipLibCheck":true,"esModuleInterop":true,"allowSyntheticDefaultImports":true,"strict":true,"forceConsistentCasingInFileNames":true,"module":"ESNext","moduleResolution":"Bundler","resolveJsonModule":true,"isolatedModules":true,"noEmit":true,"jsx":"react-jsx","baseUrl":".","paths":{"@/*":["src/*"]}},"include":["src"],"references":[]}`,
		"vite.config.ts":               `import { defineConfig } from 'vite'; import react from '@vitejs/plugin-react'; import { fileURLToPath, URL } from 'node:url'; export default defineConfig({plugins:[react()],resolve:{alias:{'@':fileURLToPath(new URL('./src',import.meta.url))}}});`,
		"playwright.config.ts":         `import {defineConfig} from '@playwright/test'; export default defineConfig({webServer:{command:'npx vite --host 127.0.0.1 --port 4173',port:4173,reuseExistingServer:false},use:{baseURL:'http://127.0.0.1:4173',browserName:'chromium'},timeout:30000});`,
		"src/components/ui/button.tsx": `import type {ButtonHTMLAttributes} from 'react'; export function Button(props:ButtonHTMLAttributes<HTMLButtonElement>){return <button type="button" {...props}/>}`,
	}
	imports, components := []string{}, []string{}
	for index, change := range product.Files {
		clean := filepath.Clean(change.Path)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || !strings.HasPrefix(filepath.ToSlash(clean), "src/generated/") || filepath.Ext(clean) != ".tsx" {
			return fmt.Errorf("생성 UI 경로가 허용 범위를 벗어났습니다: %s", change.Path)
		}
		match := componentPattern.FindStringSubmatch(change.Content)
		if len(match) != 2 {
			return fmt.Errorf("생성 UI의 export component를 찾을 수 없습니다: %s", change.Path)
		}
		alias := fmt.Sprintf("Screen%d", index)
		if len(change.ScreenIDs) == 0 {
			return fmt.Errorf("생성 UI에 화면 추적 ID가 없습니다: %s", change.Path)
		}
		modulePath := "./" + strings.TrimSuffix(filepath.ToSlash(strings.TrimPrefix(clean, "src/")), ".tsx")
		imports = append(imports, fmt.Sprintf("import { %s as %s } from %q;", match[1], alias, modulePath))
		components = append(components, alias)
		files[clean] = change.Content
	}
	files["src/main.tsx"] = strings.Join(imports, "\n") + `
import {StrictMode} from 'react'; import {createRoot} from 'react-dom/client';
const screens=[` + strings.Join(components, ",") + `];
function App(){const params=new URLSearchParams(location.search);const index=Number(params.get('screen')??0);const state=(params.get('state')??'normal') as 'normal'|'loading'|'empty'|'error'|'forbidden';const Screen=screens[index];return <StrictMode><div id="ui-validation-root"><Screen state={state}/></div></StrictMode>}
createRoot(document.getElementById('root')!).render(<App/>);`
	files["tests/ui.spec.ts"] = fmt.Sprintf(browserTest, len(components))
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

const browserTest = `import {test,expect} from '@playwright/test'; import AxeBuilder from '@axe-core/playwright';
const states=['normal','loading','empty','error','forbidden'];
const screenCount=%d;
test('render, states, interaction and semantic accessibility',async({page})=>{
  const errors:string[]=[]; page.on('pageerror',e=>errors.push(e.message));
  for(let screen=0;screen<screenCount;screen++) for(const state of states){await page.goto('/?screen='+screen+'&state='+state);await expect(page.locator('#ui-validation-root')).toBeVisible();await expect(page.locator('[data-state="'+state+'"]')).toHaveCount(1);await expect(page.locator('main h1')).toHaveCount(1);expect((await new AxeBuilder({page}).analyze()).violations).toEqual([]);}
  await page.goto('/?screen=0&state=normal');const button=page.getByRole('button',{name:'계속'}); await button.click(); await expect(page.getByText('완료되었습니다.')).toBeVisible();
  expect(errors).toEqual([]);
});
for(const width of [320,768,1280]) test('responsive '+width,async({page})=>{await page.setViewportSize({width,height:800});for(let screen=0;screen<screenCount;screen++){await page.goto('/?screen='+screen+'&state=normal');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=document.documentElement.clientWidth)).toBeTruthy();}});`
