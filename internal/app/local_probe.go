package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// Local-prompt network probes run while no session is connected. They never
// execute a user shell command: nslookup, dig and curl are implemented with
// the Go standard library, and traceroute only ever invokes the fixed system
// tool with a strictly validated host (windowless, GBK-decoded on Windows).
var localProbeSlots = make(chan struct{}, 2)

const localProbeOutputLimit = 256 << 10

type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func acquireLocalProbe() (func(), error) {
	select {
	case localProbeSlots <- struct{}{}:
		return func() { <-localProbeSlots }, nil
	default:
		return nil, errors.New("本机探测繁忙，请稍后重试")
	}
}

func localProbeHost(w http.ResponseWriter, r *http.Request, verb string) (string, bool) {
	var input struct{ Host string `json:"host"` }
	if !decode(w, r, &input) {
		return "", false
	}
	host := strings.Trim(strings.TrimSpace(input.Host), "[]")
	if !validQuickHost(host) {
		respond(w, nil, fmt.Errorf("%s 后只接受主机名或 IP", verb))
		return "", false
	}
	return host, true
}

// decodeProbeOutput keeps UTF-8 output as-is and converts GBK (the default
// console code page on zh-CN Windows) so tracert's Chinese status text such
// as 「请求超时」 no longer renders as replacement diamonds.
func decodeProbeOutput(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	if decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(raw); err == nil {
		return string(decoded)
	}
	return strings.ToValidUTF8(string(raw), "\uFFFD")
}

var digRecordTypes = map[string]bool{"A": true, "AAAA": true, "CNAME": true, "MX": true, "TXT": true, "NS": true}

func (a *App) registerLocalProbeHTTP(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/local-probe/nslookup", func(w http.ResponseWriter, r *http.Request) {
		host, ok := localProbeHost(w, r, "nslookup")
		if !ok {
			return
		}
		release, err := acquireLocalProbe()
		if err != nil {
			respond(w, nil, err)
			return
		}
		defer release()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		started := time.Now()
		var out strings.Builder
		fmt.Fprintf(&out, "本机 DNS → %s\n", host)
		if net.ParseIP(host) != nil {
			fmt.Fprintf(&out, "%s（本身即 IP 地址，无需解析）\n解析完成 · 用时 %dms", host, time.Since(started).Milliseconds())
			writeJSON(w, map[string]any{"output": out.String(), "ok": true})
			return
		}
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			respond(w, nil, fmt.Errorf("解析失败：%w", err))
			return
		}
		if len(addrs) == 0 {
			respond(w, nil, errors.New("该主机名没有可用地址记录"))
			return
		}
		for _, addr := range addrs {
			fmt.Fprintf(&out, "%s\n", addr.IP.String())
		}
		if cname, e := net.DefaultResolver.LookupCNAME(ctx, host); e == nil {
			cname = strings.TrimSuffix(cname, ".")
			if cname != "" && cname != host {
				fmt.Fprintf(&out, "规范名称：%s\n", cname)
			}
		}
		fmt.Fprintf(&out, "解析完成 · %d 条地址记录 · 用时 %dms", len(addrs), time.Since(started).Milliseconds())
		writeJSON(w, map[string]any{"output": out.String(), "ok": true})
	})
	mux.HandleFunc("POST /api/local-probe/dig", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Host string `json:"host"`
			Type string `json:"type"`
		}
		if !decode(w, r, &input) {
			return
		}
		host := strings.Trim(strings.TrimSpace(input.Host), "[]")
		if !validQuickHost(host) {
			respond(w, nil, errors.New("dig 后只接受主机名或 IP"))
			return
		}
		recordType := strings.ToUpper(strings.TrimSpace(input.Type))
		if recordType == "" {
			recordType = "A"
		}
		if !digRecordTypes[recordType] {
			respond(w, nil, errors.New("dig 支持 A / AAAA / CNAME / MX / TXT / NS 记录类型"))
			return
		}
		release, err := acquireLocalProbe()
		if err != nil {
			respond(w, nil, err)
			return
		}
		defer release()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		started := time.Now()
		var out strings.Builder
		fmt.Fprintf(&out, "; <<>> DevShell dig <<>> %s %s\n;; QUESTION SECTION:\n;%s.\t\tIN\t%s\n", host, recordType, host, recordType)
		answers := make([]string, 0, 8)
		switch recordType {
		case "A", "AAAA":
			if net.ParseIP(host) != nil {
				if (recordType == "A") == (net.ParseIP(host).To4() != nil) {
					answers = append(answers, fmt.Sprintf("%s.\tIN\t%s\t%s", host, recordType, host))
				}
			} else {
				addrs, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, host)
				if lookupErr != nil {
					respond(w, nil, fmt.Errorf("查询失败：%w", lookupErr))
					return
				}
				for _, addr := range addrs {
					if (recordType == "A") == (addr.IP.To4() != nil) {
						answers = append(answers, fmt.Sprintf("%s.\tIN\t%s\t%s", host, recordType, addr.IP.String()))
					}
				}
			}
		case "CNAME":
			cname, lookupErr := net.DefaultResolver.LookupCNAME(ctx, host)
			if lookupErr != nil {
				respond(w, nil, fmt.Errorf("查询失败：%w", lookupErr))
				return
			}
			cname = strings.TrimSuffix(cname, ".")
			if cname != "" && cname != host {
				answers = append(answers, fmt.Sprintf("%s.\tIN\tCNAME\t%s.", host, cname))
			}
		case "MX":
			mx, lookupErr := net.DefaultResolver.LookupMX(ctx, host)
			if lookupErr != nil {
				respond(w, nil, fmt.Errorf("查询失败：%w", lookupErr))
				return
			}
			for _, record := range mx {
				answers = append(answers, fmt.Sprintf("%s.\tIN\tMX\t%d %s.", host, record.Pref, strings.TrimSuffix(record.Host, ".")))
			}
		case "TXT":
			texts, lookupErr := net.DefaultResolver.LookupTXT(ctx, host)
			if lookupErr != nil {
				respond(w, nil, fmt.Errorf("查询失败：%w", lookupErr))
				return
			}
			for _, text := range texts {
				answers = append(answers, fmt.Sprintf("%s.\tIN\tTXT\t%q", host, text))
			}
		case "NS":
			ns, lookupErr := net.DefaultResolver.LookupNS(ctx, host)
			if lookupErr != nil {
				respond(w, nil, fmt.Errorf("查询失败：%w", lookupErr))
				return
			}
			for _, record := range ns {
				answers = append(answers, fmt.Sprintf("%s.\tIN\tNS\t%s.", host, strings.TrimSuffix(record.Host, ".")))
			}
		}
		out.WriteString(";; ANSWER SECTION:\n")
		if len(answers) == 0 {
			out.WriteString("(无记录)")
		} else {
			out.WriteString(strings.Join(answers, "\n"))
		}
		fmt.Fprintf(&out, "\n\n;; 本机 DNS · 用时 %dms", time.Since(started).Milliseconds())
		writeJSON(w, map[string]any{"output": out.String(), "ok": true})
	})
	mux.HandleFunc("POST /api/local-probe/traceroute", func(w http.ResponseWriter, r *http.Request) {
		host, ok := localProbeHost(w, r, "traceroute")
		if !ok {
			return
		}
		release, err := acquireLocalProbe()
		if err != nil {
			respond(w, nil, err)
			return
		}
		defer release()
		name, args := "traceroute", []string{"-n", "-m", "20", "-w", "2", host}
		if runtime.GOOS == "windows" {
			name, args = "tracert", []string{"-d", "-h", "20", "-w", "1000", host}
		}
		path, err := exec.LookPath(name)
		if err != nil {
			respond(w, nil, fmt.Errorf("本机未找到 %s 命令", name))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		buffer := &limitedBuffer{limit: localProbeOutputLimit}
		command := exec.CommandContext(ctx, path, args...)
		hideProbeWindow(command)
		command.Stdout = buffer
		command.Stderr = buffer
		runErr := command.Run()
		output := strings.TrimRight(decodeProbeOutput(buffer.buf.Bytes()), "\r\n")
		if output == "" && runErr != nil {
			respond(w, nil, fmt.Errorf("%s 执行失败：%v", name, runErr))
			return
		}
		if ctx.Err() != nil {
			output += "\n（已达到 45 秒上限，提前停止）"
		}
		writeJSON(w, map[string]any{"output": output, "ok": runErr == nil})
	})
	mux.HandleFunc("POST /api/local-probe/curl", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			URL  string `json:"url"`
			Head bool   `json:"head"`
		}
		if !decode(w, r, &input) {
			return
		}
		target, err := parseLocalProbeURL(input.URL)
		if err != nil {
			respond(w, nil, err)
			return
		}
		release, err := acquireLocalProbe()
		if err != nil {
			respond(w, nil, err)
			return
		}
		defer release()
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		method := http.MethodGet
		if input.Head {
			method = http.MethodHead
		}
		request, err := http.NewRequestWithContext(ctx, method, target, nil)
		if err != nil {
			respond(w, nil, errors.New("网址无效"))
			return
		}
		request.Header.Set("User-Agent", "curl/8.0 (DevShell local prompt)")
		client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, err := client.Do(request)
		if err != nil {
			respond(w, nil, fmt.Errorf("请求失败：%v", err))
			return
		}
		defer response.Body.Close()
		var out strings.Builder
		fmt.Fprintf(&out, "%s %s\n", response.Proto, response.Status)
		response.Header.Write(&out)
		if !input.Head {
			out.WriteString("\r\n")
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
			if readErr != nil {
				fmt.Fprintf(&out, "（读取响应体失败：%v）", readErr)
			} else {
				out.WriteString(strings.ToValidUTF8(string(body), "\uFFFD"))
				if len(body) == 64<<10 {
					out.WriteString("\n…（响应体已截断）")
				}
			}
		}
		writeJSON(w, map[string]any{"output": strings.TrimRight(out.String(), "\r\n"), "ok": response.StatusCode < 400})
	})
}

// parseLocalProbeURL mirrors the real curl tool: a scheme-less target such as
// "ip.sb" is accepted and defaults to http (curl.se URL syntax: prefix guesses
// like ftp. aside, "all other means HTTP"); explicit schemes must be http/https.
func parseLocalProbeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 || strings.ContainsAny(raw, " \t\r\n") {
		return "", errors.New("curl 后需要一个网址（不带协议时默认 http）")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("curl 仅支持 http/https 网址")
	}
	return parsed.String(), nil
}
