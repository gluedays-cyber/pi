package internal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func runPythonFile(filename string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, PythonCmd, filename)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	// 임시 디렉터리 경로 대신, 명령을 실행한 원래의 작업 디렉터리 경로를 지정
	cmd.Dir = "."

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return stdout.String(), stderr.String(), fmt.Errorf("실행 타임아웃 초과 (30초)")
	}

	if err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return stdout.String(), stderr.String(), fmt.Errorf("파이썬 실행 파일을 찾을 수 없음: %s가 올바른 경로인지 확인 필요", PythonCmd)
		}
		if _, ok := err.(*exec.ExitError); !ok {
			return stdout.String(), stderr.String(), fmt.Errorf("파이썬 인터프리터 시작 실패: %w", err)
		}
	}
	return stdout.String(), stderr.String(), err
}

func validatePythonSyntax(filename string) error {
	cmd := exec.Command(PythonCmd, "-c", "import ast, sys; ast.parse(open(sys.argv[1], encoding='utf-8').read())", filename)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return fmt.Errorf("파이썬 실행 파일을 찾을 수 없음: %s", PythonCmd)
		}
		if _, ok := err.(*exec.ExitError); !ok {
			return fmt.Errorf("문법 검사 시작 실패: %w", err)
		}
		return fmt.Errorf("SyntaxError: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

func InstallRequirements() error {
	if _, err := os.Stat("requirements.txt"); os.IsNotExist(err) {
		return nil
	}
	Log.Info("[requirements.txt 감지됨. 의존성 패키지 자동 설치 중...]\n")
	cmd := exec.Command(PythonCmd, "-m", "pip", "install", "-r", "requirements.txt")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return fmt.Errorf("파이썬 및 pip 실행 파일을 찾을 수 없음: %s", PythonCmd)
		}
		return fmt.Errorf("의존성 패키지 설치 실패: %w", err)
	}
	return nil
}
