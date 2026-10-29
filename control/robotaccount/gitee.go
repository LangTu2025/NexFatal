package robotaccount

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultGiteeAPI = "https://gitee.com/api/v5"

var errGiteeConflict = errors.New("Gitee 文件内容发生冲突")

type giteeStatusError struct {
	status int
}

func (e *giteeStatusError) Error() string {
	return fmt.Sprintf("Gitee API 返回 HTTP %d", e.status)
}

type safeRequestError struct {
	operation string
	cause     error
}

func (e *safeRequestError) Error() string {
	return e.operation + "失败"
}

func (e *safeRequestError) Unwrap() error {
	return e.cause
}

type giteeFile struct {
	Content string
	SHA     string
}

type giteeClient struct {
	baseURL    string
	owner      string
	repository string
	branch     string
	token      string
	httpClient *http.Client
}

func newDefaultGiteeClient() *giteeClient {
	return &giteeClient{
		baseURL:    defaultGiteeAPI,
		owner:      giteeOwner,
		repository: giteeRepository,
		branch:     giteeBranch,
		token:      giteeAccessToken,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

func (c *giteeClient) contentURL(path string) (string, error) {
	base, err := url.Parse(strings.TrimRight(c.baseURL, "/"))
	if err != nil {
		return "", errors.New("Gitee API 地址无效")
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	base.Path += "/repos/" + url.PathEscape(c.owner) + "/" + url.PathEscape(c.repository) + "/contents/" + strings.Join(parts, "/")
	return base.String(), nil
}

func (c *giteeClient) getFile(ctx context.Context, path string) (giteeFile, error) {
	endpoint, err := c.contentURL(path)
	if err != nil {
		return giteeFile{}, err
	}
	query := url.Values{"access_token": {c.token}, "ref": {c.branch}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return giteeFile{}, errors.New("无法创建 Gitee 读取请求")
	}
	c.setAuthenticationHeaders(request)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return giteeFile{}, &safeRequestError{operation: "读取 Gitee 账号池", cause: unwrapURLError(err)}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return giteeFile{}, &giteeStatusError{status: response.StatusCode}
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return giteeFile{}, &safeRequestError{operation: "读取 Gitee 响应", cause: err}
	}
	var payload struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return giteeFile{}, errors.New("Gitee 文件响应格式无效")
	}
	if payload.Encoding != "" && !strings.EqualFold(payload.Encoding, "base64") {
		return giteeFile{}, errors.New("Gitee 文件编码不受支持")
	}

	decoded, err := base64.StdEncoding.DecodeString(removeBase64Newlines(payload.Content))
	if err != nil {
		return giteeFile{}, errors.New("Gitee 文件内容无效")
	}
	return giteeFile{Content: string(decoded), SHA: payload.SHA}, nil
}

func (c *giteeClient) putFile(ctx context.Context, path, content, sha string) error {
	endpoint, err := c.contentURL(path)
	if err != nil {
		return err
	}
	payload := struct {
		AccessToken string `json:"access_token"`
		Content     string `json:"content"`
		SHA         string `json:"sha"`
		Message     string `json:"message"`
		Branch      string `json:"branch"`
	}{
		AccessToken: c.token,
		Content:     base64.StdEncoding.EncodeToString([]byte(content)),
		SHA:         sha,
		Message:     "remove banned login account",
		Branch:      c.branch,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.New("无法编码 Gitee 更新内容")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("无法创建 Gitee 更新请求")
	}
	request.Header.Set("Content-Type", "application/json")
	c.setAuthenticationHeaders(request)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return &safeRequestError{operation: "更新 Gitee 账号池", cause: unwrapURLError(err)}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode == http.StatusConflict || response.StatusCode == http.StatusUnprocessableEntity {
		return errGiteeConflict
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &giteeStatusError{status: response.StatusCode}
	}
	return nil
}

func (c *giteeClient) getReadableFile(ctx context.Context, path string) (giteeFile, error) {
	file, apiErr := c.getFile(ctx, path)
	if apiErr == nil || strings.TrimRight(c.baseURL, "/") != defaultGiteeAPI {
		return file, apiErr
	}
	endpoint := fmt.Sprintf(
		"https://gitee.com/%s/%s/raw/%s/%s",
		url.PathEscape(c.owner),
		url.PathEscape(c.repository),
		url.PathEscape(c.branch),
		strings.TrimLeft(path, "/"),
	)
	query := url.Values{"access_token": {c.token}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return giteeFile{}, apiErr
	}
	c.setAuthenticationHeaders(request)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return giteeFile{}, apiErr
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return giteeFile{}, apiErr
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return giteeFile{}, apiErr
	}
	return giteeFile{Content: string(body)}, nil
}

func (c *giteeClient) setAuthenticationHeaders(request *http.Request) {
	if strings.TrimSpace(c.token) == "" {
		return
	}
	request.Header.Set("Authorization", "token "+c.token)
	request.Header.Set("Private-Token", c.token)
}

func (c *giteeClient) deleteExactLine(ctx context.Context, path, target string) error {
	for attempt := 0; attempt < 3; attempt++ {
		current, err := c.getFile(ctx, path)
		if err != nil {
			return err
		}
		updated, found := removeExactLine(current.Content, target)
		if !found {
			return nil
		}
		err = c.putFile(ctx, path, updated, current.SHA)
		if errors.Is(err, errGiteeConflict) {
			continue
		}
		return err
	}
	return errGiteeConflict
}

func removeBase64Newlines(content string) string {
	return strings.Map(func(character rune) rune {
		if character == '\r' || character == '\n' {
			return -1
		}
		return character
	}, content)
}

func removeExactLine(content, target string) (string, bool) {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	target = strings.TrimSuffix(strings.TrimSuffix(target, "\n"), "\r")
	hadTrailingNewline := strings.HasSuffix(normalized, "\n")
	lines := strings.Split(normalized, "\n")
	if hadTrailingNewline {
		lines = lines[:len(lines)-1]
	}
	for index, line := range lines {
		if line != target {
			continue
		}
		lines = append(lines[:index], lines[index+1:]...)
		result := strings.Join(lines, newline)
		if hadTrailingNewline && len(lines) > 0 {
			result += newline
		}
		return result, true
	}
	return content, false
}

func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err
	}
	return err
}
