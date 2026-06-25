package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pi/pkg/fs"
	"pi/pkg/logger"
	"pi/pkg/metrics"
)

var Log logger.Logger = logger.Log

func ExecutePythonWorkflow(pyCode string, input string, chatHistory []Content, aiResponse string, primaryModel string) (bool, string, []Content, error) {
	filename, err := writeTempPythonFile(pyCode)
	if err != nil {
		return false, "", nil, err
	}
	defer os.Remove(filename)

	Log.Info("[Python 코드 실행 중...]\n")
	if err := InstallRequirements(); err != nil {
		Log.Warn("[의존성 설치 실패] %v\n", err)
	}

	beforeState, _ := fs.GetDirState(".", "py-cli-")

	var stdout, stderr string
	var execErr error
	if syntaxErr := validatePythonSyntax(filename); syntaxErr != nil {
		execErr = syntaxErr
		stderr = syntaxErr.Error()
	} else {
		stdout, stderr, execErr = runPythonFile(filename)
	}

	logExecutionHistory(pyCode, stdout, stderr, execErr)

	afterState, _ := fs.GetDirState(".", "py-cli-")
	diff := fs.CompareDirStates(beforeState, afterState)
	printFSChanges(diff)

	printExecutionOutputs(stdout, stderr, execErr)

	feedbackPrompt := fmt.Sprintf(
		"작성한 파이썬 코드 실행 결과는 다음과 같음.\n\n[표준 출력]\n%s\n\n[표준 에러]\n%s\n\n[실행 오류]\n%v\n\n결과를 요약하고 피드백할 것.",
		stdout, stderr, execErr,
	)

	feedbackResponse, err := callGeminiFeedback(primaryModel, chatHistory, aiResponse, feedbackPrompt)
	if err != nil {
		return false, "", nil, err
	}

	if execErr != nil {
		updatedHistory := buildRetryHistory(feedbackResponse, input, stderr, execErr)
		metrics.RecordRun(false)
		metrics.PrintStats()
		return false, "", updatedHistory, nil
	}

	metrics.RecordRun(true)
	metrics.PrintStats()
	return true, feedbackResponse, nil, nil
}

func writeTempPythonFile(pyCode string) (string, error) {
	tmpFile, err := os.CreateTemp("", "py-cli-*.py")
	if err != nil {
		return "", fmt.Errorf("임시 파일 생성 오류: %w", err)
	}
	filename := filepath.Clean(tmpFile.Name())

	if filename == "" || filename == "." || filename == "/" {
		tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("유효하지 않은 임시 파일 경로: %s", filename)
	}

	if err := tmpFile.Chmod(0644); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("파일 권한 설정 오류: %w", err)
	}

	if _, err := tmpFile.Write([]byte(pyCode)); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("파일 저장 오류: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("파일 동기화 오류: %w", err)
	}
	tmpFile.Close()
	return filename, nil
}

func printFSChanges(diff fs.FsDiff) {
	var fsChanges strings.Builder
	if len(diff.Created) > 0 {
		fsChanges.WriteString(fmt.Sprintf("- 생성된 파일: %s\n", strings.Join(diff.Created, ", ")))
	}
	if len(diff.Modified) > 0 {
		fsChanges.WriteString(fmt.Sprintf("- 변경된 파일: %s\n", strings.Join(diff.Modified, ", ")))
	}
	if len(diff.Deleted) > 0 {
		fsChanges.WriteString(fmt.Sprintf("- 삭제된 파일: %s\n", strings.Join(diff.Deleted, ", ")))
	}
	fsChangesStr := fsChanges.String()
	if fsChangesStr == "" {
		fsChangesStr = "없음\n"
	}
}

func printExecutionOutputs(stdout, stderr string, execErr error) {
	Log.Info("\n--- [Python 실행 출력] ---\n")
	if stdout != "" {
		Log.Info("[표준 출력]\n%s", stdout)
	}
	if stderr != "" {
		Log.Error("[표준 에러]\n%s", stderr)
	}
	if execErr != nil {
		Log.Error("[실행 오류] %v\n", execErr)
	}
	Log.Info("---------------------------\n")
}

func callGeminiFeedback(primaryModel string, chatHistory []Content, aiResponse string, feedbackPrompt string) (string, error) {
	tempHistory := append(chatHistory, Content{
		Role:  "model",
		Parts: []Part{{Text: aiResponse}},
	})
	tempHistory = append(tempHistory, Content{
		Role:  "user",
		Parts: []Part{{Text: feedbackPrompt}},
	})

	Log.Info("AI 실행 결과 분석 중...\n")
	var feedbackResponse string
	var callErr error
	for attempt := 1; attempt <= 3; attempt++ {
		feedbackResponse, callErr = CallGemini(primaryModel, tempHistory)
		if callErr == nil {
			break
		}
		Log.Warn("[API 오류 감지] %s 모델 호출 실패(피드백 시도 %d/3): %v. 재시도 진행...\n", primaryModel, attempt, callErr)
		if attempt < 3 {
			time.Sleep(2 * time.Second)
		}
	}
	if callErr != nil {
		return "", fmt.Errorf("API 최종 오류 (피드백): %w", callErr)
	}
	return feedbackResponse, nil
}

func buildRetryHistory(feedbackResponse, input, stderr string, execErr error) []Content {
	Log.Info("\nAI 피드백 >\n")
	Log.Info("%s\n", feedbackResponse)
	Log.Error("\n[오류 발생] 파이썬 코드 실행 중 에러가 발생했다.\n")

	errMsg := stderr
	if errMsg == "" {
		errMsg = execErr.Error()
	}
	errMsg = strings.TrimSpace(errMsg)

	retryPrompt := fmt.Sprintf(
		"사용자는 '%s'을 원하는데, 파이썬 코드를 작성하여 실행한 결과, '%s'가 발생했으니, 파이썬 코드를 다시 작성해서 시도해야 한다.",
		input, errMsg,
	)
	Log.Warn("\n[오류 감지] 아래 프롬프트로 재실행한다:\n%s\n\n", retryPrompt)
	return []Content{
		{
			Role:  "user",
			Parts: []Part{{Text: retryPrompt}},
		},
	}
}
