package middleware

import (
	"bytes"
	"fmt"
	"genspark2api/model"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

// 全局HTTP客户端，启用连接复用
var httpClient = &http.Client{}

func ForwardTo(c *gin.Context, baseURL string) {
	// 复制请求体到缓冲区，同时避免修改原始请求体
	var bodyBytes bytes.Buffer
	if _, err := io.Copy(&bodyBytes, c.Request.Body); err != nil {
		respondWithError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 重设请求体，使其可以再次被读取
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes.Bytes()))

	// 构造新的目标URL
	targetURL := buildTargetURL(baseURL, c.Request.URL.Path, c.Request.URL.RawQuery)

	// 创建新的请求
	newReq, err := http.NewRequest(c.Request.Method, targetURL, bytes.NewReader(bodyBytes.Bytes()))
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 复制请求头
	copyHeaders(c.Request.Header, newReq.Header)

	// 发送请求
	resp, err := httpClient.Do(newReq)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Body.Close()

	// 将响应内容写回原始请求的响应中
	if err := transferResponse(c.Writer, resp); err != nil {
		respondWithError(c, http.StatusInternalServerError, err.Error())
	}
}

func buildTargetURL(baseURL, path, query string) string {
	target := baseURL + path
	if query != "" {
		target += "?" + query
	}
	return target
}

func copyHeaders(source, destination http.Header) {
	for key, values := range source {
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func transferResponse(w gin.ResponseWriter, resp *http.Response) error {
	// 设置响应头和状态码
	for key, value := range resp.Header {
		w.Header()[key] = value
	}
	w.WriteHeader(resp.StatusCode)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("expected http.ResponseWriter to be an http.Flusher")
	}

	buf := make([]byte, 1024) // 使用一个缓冲区来读取数据
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := w.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			flusher.Flush() // 立即将缓冲的数据发送出去
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func respondWithError(c *gin.Context, code int, message string) {
	c.JSON(code, model.OpenAIErrorResponse{
		OpenAIError: model.OpenAIError{
			Message: message,
			Type:    "request_error",
			Code:    "FORWARD_ERR",
		},
	})
}
