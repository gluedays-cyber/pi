package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

var ApiKey string

func getApiURL(model string) string {
	return "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":generateContent?key=" + ApiKey
}

type Content struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GenerateRequest struct {
	Contents          []Content          `json:"contents"`
	SystemInstruction *SystemInstruction `json:"system_instruction,omitempty"`
}

type SystemInstruction struct {
	Parts []Part `json:"parts"`
}

type GenerateResponse struct {
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Content Content `json:"content"`
}

func ManageHistory(history []Content) []Content {
	const maxThreshold = 8
	if len(history) <= maxThreshold {
		return history
	}

	managed := make([]Content, 0, maxThreshold)
	// 최초 사용자 요청 보존
	managed = append(managed, history[0])

	// 최초 AI 응답도 보존하여 초기 흐름 유지
	if len(history) > 1 && history[1].Role == "model" {
		managed = append(managed, history[1])
	}

	// 최근 대화 내용 위주로 슬라이딩 윈도우 구성
	startIdx := len(history) - 4
	if startIdx < len(managed) {
		startIdx = len(managed)
	}

	for i := startIdx; i < len(history); i++ {
		managed = append(managed, history[i])
	}

	return managed
}

func CallGemini(model string, history []Content) (string, error) {
	reqBody := GenerateRequest{
		Contents: ManageHistory(history),
		SystemInstruction: &SystemInstruction{
			Parts: []Part{{Text: "당신은 파이썬 코드를 생성하고 실행 결과를 피드백하는 개발 보조 AI다. 파이썬 코드를 포함할 때는 반드시 ```python ... ``` 코드 블록 형식으로 제공해야 한다.\n\n[라이브러리 임포트 오류 시 자동 해결 방안]\n외부 라이브러리 사용 시 파이썬 코드 내에서 작업 디렉터리에 requirements.txt 파일을 생성하고 필요한 패키지 목록을 기입하도록 작성하여 의존성이 자동 설치될 수 있도록 조치해야 한다. (예: 코드 내에서 open('requirements.txt', 'w') 등으로 패키지명 작성)\n\n[데이터 출력 형식 지정]\n1. 모든 데이터 및 리포트 출력은 구조화된 가로 표(table) 형식, 정돈된 JSON, 또는 key-value 쌍을 이용한 줄바꿈 구조로 포맷팅하여 표준 출력(print)해야 한다.\n2. 마크다운(markdown) 태그를 사용하지 말고, 일반 텍스트(plain text)와 특수문자(|, -, : 등), 줄바꿈, 들여쓰기 등의 단순한 포맷만을 사용하여 가독성을 극대화해야 한다.\n\n사용자가 로컬 디렉터리의 파일을 읽거나 구조를 탐색하길 원한다면, 파이썬의 파일 입출력 코드(예: open('파일명', 'r', encoding='utf-8')을 활용한 텍스트 출력)나 os 모듈 코드(예: os.listdir())를 적극적으로 작성하여 표준 출력(print)으로 내용을 보여주도록 유도해야 한다. 답변할 때는 '입니다', '합니다' 등을 생략하고 서술어가 없는 명사로 문장을 마무리하거나, '이다', '한다' 등의 간략한 표현을 사용해야 한다."}},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", getApiURL(model), bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status: %s, body: %s", resp.Status, string(bodyBytes))
	}

	var resObj GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&resObj); err != nil {
		return "", err
	}

	if len(resObj.Candidates) > 0 && len(resObj.Candidates[0].Content.Parts) > 0 {
		return resObj.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("empty response from Gemini")
}

func ExtractPythonCode(response string) string {
	startToken := "```python"
	endToken := "```"

	startIndex := strings.Index(response, startToken)
	if startIndex == -1 {
		return ""
	}
	startIndex += len(startToken)

	endIndex := strings.Index(response[startIndex:], endToken)
	if endIndex == -1 {
		return ""
	}
	endIndex += startIndex

	return strings.TrimSpace(response[startIndex:endIndex])
}

func SaveSession(history []Content) error {
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("session.json", data, 0644)
}

func LoadSession() ([]Content, error) {
	data, err := os.ReadFile("session.json")
	if err != nil {
		return nil, err
	}
	var history []Content
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	return history, nil
}
