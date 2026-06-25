package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var PythonCmd, IsVenv = resolvePythonInterpreter()

func resolvePythonInterpreter() (string, bool) {
	var pythonPath string

	if cfg, err := LoadConfig(); err == nil && cfg.PythonPath != "" {
		if _, err := os.Stat(cfg.PythonPath); err == nil {
			pythonPath = cfg.PythonPath
		}
	}

	if pythonPath == "" {
		if envPath := os.Getenv("VIRTUAL_ENV"); envPath != "" {
			winPath := filepath.Join(envPath, "Scripts", "python.exe")
			unixPath := filepath.Join(envPath, "bin", "python")
			if _, err := os.Stat(winPath); err == nil {
				pythonPath = winPath
			} else if _, err := os.Stat(unixPath); err == nil {
				pythonPath = unixPath
			}
		}
	}

	if pythonPath == "" {
		if envPath := os.Getenv("PI_PYTHON"); envPath != "" {
			pythonPath = envPath
		} else if envPath := os.Getenv("PYTHON_INTERPRETER"); envPath != "" {
			pythonPath = envPath
		}
	}

	if pythonPath == "" {
		candidates := []string{
			".venv/Scripts/python.exe",
			"venv/Scripts/python.exe",
			".venv/bin/python",
			"venv/bin/python",
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				pythonPath = path
				break
			}
		}
	}

	if pythonPath == "" {
		pythonPath = "python"
	}

	isVenv := checkVenv(pythonPath)
	return pythonPath, isVenv
}

func checkVenv(pythonPath string) bool {
	// sys.prefix 디렉토리 내에 pyvenv.cfg 가 존재하고 격리된 환경인지 교차 검증
	script := "import sys, os; print(((sys.prefix != sys.base_prefix) or hasattr(sys, 'real_prefix')) and os.path.exists(os.path.join(sys.prefix, 'pyvenv.cfg')))"
	cmd := exec.Command(pythonPath, "-c", script)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "True"
}

func GetPythonVersion(pythonPath string) (string, error) {
	cmd := exec.Command(pythonPath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
