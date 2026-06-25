package app

import (
	"bufio"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"pi/internal"
	"pi/pkg/coverage"
	"pi/pkg/depgraph"
	"pi/pkg/logger"
	"pi/pkg/metrics"
)

func Run() error {
	defer internal.Cleanup()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		internal.Cleanup()
		os.Exit(0)
	}()

	printBanner()
	if err := validateVirtualEnvironment(); err != nil {
		return err
	}

	cfg, err := internal.LoadConfig()
	if err != nil {
		logger.Log.Warn("설정 로드 실패. 기본 구성을 활용해 지속함: %v\n", err)
		cfg = &internal.DecryptedConfig{}
	}

	if err := getAPIKey(cfg); err != nil {
		return err
	}

	primaryModel := cfg.Model1
	if primaryModel == "" {
		primaryModel = "gemini-2.5-flash-lite"
	}

	logger.Log.Info("[사용 모델]: %s\n", primaryModel)

	if len(os.Args) >= 2 {
		input := strings.TrimSpace(strings.Join(os.Args[1:], " "))
		if input != "" {
			executeWorkflow(input, primaryModel, nil)
			return nil
		}
	}

	runREPLLoop(primaryModel)
	return nil
}

func runREPLLoop(primaryModel string) {
	logger.Log.Info("\n대화식 입력 모드로 진입했다. (종료: 'exit'/'q', 지표: 'stats', 커버리지: 'coverage', 의존성: 'depgraph', 도움말: 'help'/'h', 저장: 'save', 불러오기: 'load')\n")
	reader := bufio.NewReader(os.Stdin)
	var activeHistory []internal.Content

	for {
		logger.Log.Info("\npi > ")
		input, err := reader.ReadString('\n')
		if err != nil {
			logger.Log.Error("입력 리딩 에러: %v\n", err)
			continue
		}
		input = strings.TrimSpace(input)

		var exit bool
		activeHistory, exit = handleREPLCommand(input, reader, activeHistory, primaryModel)
		if exit {
			break
		}
	}
}

func handleREPLCommand(input string, reader *bufio.Reader, activeHistory []internal.Content, primaryModel string) ([]internal.Content, bool) {
	if input == "exit" || input == "q" {
		return activeHistory, true
	}
	if input == "stats" {
		metrics.PrintStats()
		return activeHistory, false
	}
	if input == "coverage" {
		coverage.PrintCoverageReport()
		return activeHistory, false
	}
	if input == "depgraph" {
		depgraph.PrintDepGraph()
		return activeHistory, false
	}
	if input == "help" || input == "h" {
		printHelp()
		return activeHistory, false
	}
	if input == "save" {
		if err := internal.SaveSession(activeHistory); err != nil {
			logger.Log.Error("세션 저장 실패: %v\n", err)
		} else {
			logger.Log.Success("대화 세션 저장 완료 (session.json)\n")
		}
		return activeHistory, false
	}
	if input == "load" {
		history, err := internal.LoadSession()
		if err != nil {
			logger.Log.Error("세션 불러오기 실패: %v\n", err)
		} else {
			activeHistory = history
			logger.Log.Success("대화 세션 불러오기 완료 (%d개 메세지 복구됨)\n", len(activeHistory))
		}
		return activeHistory, false
	}
	if input == "" {
		return activeHistory, false
	}
	activeHistory = executeWorkflow(input, primaryModel, activeHistory)
	return activeHistory, false
}

func executeWorkflow(input string, primaryModel string, chatHistory []internal.Content) []internal.Content {
	updatedHistory, feedback, isCodeWorkflow, err := internal.ExecuteWorkflow(input, primaryModel, chatHistory)
	if err != nil {
		logger.Log.Error("%v. 복구 가능한 에러로 취급하여 메인 대기 모드로 돌아감.\n", err)
		return updatedHistory
	}

	if isCodeWorkflow {
		logger.Log.Success("\n==============================================\n")
		logger.Log.Success("성공적으로 작업을 수행했다.\n")
		logger.Log.Success("수행된 작업 요약:\n%s\n\n", feedback)
		logger.Log.Success("==============================================\n")
	} else {
		logger.Log.Info("\nAI 응답 >\n")
		logger.Log.Info("%s\n", feedback)
	}

	return updatedHistory
}
