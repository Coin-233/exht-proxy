package proxy

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const warpRepository = "https://pkg.cloudflareclient.com/"

func ensureWarpCLI(ctx context.Context, binary string) (string, error) {
	if binary == "" {
		binary = "warp-cli"
	}
	if path, err := exec.LookPath(binary); err == nil {
		return path, nil
	}
	if binary != "warp-cli" {
		return "", fmt.Errorf("找不到配置的 WARP_CLI: %s", binary)
	}
	local, err := filepath.Abs("warp-cli")
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(local); err == nil {
		if info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			return local, nil
		}
		return "", fmt.Errorf("%s 已存在但不是可执行文件", local)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return "", fmt.Errorf("WARP 自动下载仅支持 Linux amd64/arm64, 当前为 %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if _, err := exec.LookPath("tar"); err != nil {
		return "", fmt.Errorf("提取 WARP 需要 tar: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 3 * time.Minute}
	log.Println("未安装 WARP, 正在从 Cloudflare 官方仓库下载到当前目录")
	metadata, err := warpGet(ctx, client, warpRepository+"dists/bookworm/main/binary-"+runtime.GOARCH+"/Packages.gz")
	if err != nil {
		return "", err
	}
	zipped, err := gzip.NewReader(metadata)
	if err != nil {
		metadata.Close()
		return "", err
	}
	index, err := io.ReadAll(io.LimitReader(zipped, 8<<20))
	zipped.Close()
	metadata.Close()
	if err != nil {
		return "", err
	}
	pkg, err := parseWarpPackage(string(index), runtime.GOARCH)
	if err != nil {
		return "", err
	}
	temp, err := os.MkdirTemp(".", ".warp-download-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	body, err := warpGet(ctx, client, warpRepository+pkg["Filename"])
	if err != nil {
		return "", err
	}
	archive, err := os.Create(filepath.Join(temp, "warp.deb"))
	if err != nil {
		body.Close()
		return "", err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(body, 256<<20))
	body.Close()
	closeErr := archive.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	expectedSize, _ := strconv.ParseInt(pkg["Size"], 10, 64)
	if n != expectedSize || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), pkg["SHA256"]) {
		return "", fmt.Errorf("WARP 下载大小或 SHA256 校验失败")
	}
	data, err := extractDebData(filepath.Join(temp, "warp.deb"), temp)
	if err != nil {
		return "", err
	}
	listing, err := exec.CommandContext(ctx, "tar", "-tf", data).Output()
	if err != nil {
		return "", fmt.Errorf("读取 WARP 软件包失败: %w", err)
	}
	for _, name := range []string{"warp-cli", "warp-svc"} {
		member := ""
		for _, line := range strings.Split(string(listing), "\n") {
			if strings.TrimPrefix(line, "./") == "bin/"+name || strings.TrimPrefix(line, "./") == "usr/bin/"+name {
				member = line
				break
			}
		}
		if member == "" {
			return "", fmt.Errorf("官方包中缺少 %s", name)
		}
		out, err := os.OpenFile(filepath.Join(temp, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if err != nil {
			return "", err
		}
		cmd := exec.CommandContext(ctx, "tar", "-xOf", data, "--", member)
		cmd.Stdout = out
		runErr := cmd.Run()
		closeErr := out.Close()
		if runErr != nil {
			return "", runErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	published := []string{}
	complete := false
	defer func() {
		if !complete {
			for _, name := range published {
				_ = os.Remove(name)
			}
		}
	}()
	for _, name := range []string{"warp-svc", "warp-cli"} {
		if err := os.Link(filepath.Join(temp, name), name); err != nil {
			return "", fmt.Errorf("保存 ./%s 失败: %w", name, err)
		}
		published = append(published, name)
	}
	complete = true
	log.Println("WARP 下载完成：./warp-cli 和 ./warp-svc")
	return local, nil
}

func warpGet(ctx context.Context, client *http.Client, address string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("WARP 下载 HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func parseWarpPackage(index, arch string) (map[string]string, error) {
	for _, block := range strings.Split(index, "\n\n") {
		fields := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			if key, value, ok := strings.Cut(line, ": "); ok {
				fields[key] = value
			}
		}
		if fields["Package"] != "cloudflare-warp" || fields["Architecture"] != arch {
			continue
		}
		filename := fields["Filename"]
		size, err := strconv.ParseInt(fields["Size"], 10, 64)
		digest, digestErr := hex.DecodeString(fields["SHA256"])
		if !strings.HasPrefix(filename, "pool/") || strings.Contains(filename, "..") || strings.ContainsAny(filename, "?#\\") || err != nil || size <= 0 || size > 256<<20 || digestErr != nil || len(digest) != 32 {
			return nil, fmt.Errorf("WARP 仓库元数据无效")
		}
		return fields, nil
	}
	return nil, fmt.Errorf("官方仓库未找到 cloudflare-warp (%s)", arch)
}

func extractDebData(filename, directory string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	magic := make([]byte, 8)
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != "!<arch>\n" {
		return "", fmt.Errorf("无效的 Debian 软件包")
	}
	for {
		header := make([]byte, 60)
		if _, err := io.ReadFull(reader, header); err != nil {
			return "", fmt.Errorf("软件包缺少 data.tar: %w", err)
		}
		size, err := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
		if err != nil || size < 0 || size > 256<<20 || string(header[58:]) != "`\n" {
			return "", fmt.Errorf("无效的 ar 成员")
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(header[:16])), "/")
		if name == "data.tar.xz" || name == "data.tar.zst" || name == "data.tar.gz" || name == "data.tar" {
			target := filepath.Join(directory, name)
			out, err := os.Create(target)
			if err != nil {
				return "", err
			}
			_, copyErr := io.CopyN(out, reader, size)
			closeErr := out.Close()
			if copyErr != nil {
				return "", copyErr
			}
			if closeErr != nil {
				return "", closeErr
			}
			return target, nil
		}
		if _, err := io.CopyN(io.Discard, reader, size+size%2); err != nil {
			return "", err
		}
	}
}
