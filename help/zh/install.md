# 安装 BuildMax

BuildMax 提供三个二进制文件。`buildmax` 是本地 CLI/TUI，也是你开始所需的全部；
`buildmax-server` 和 `buildmax-worker` 用于 space 部署。

在选择安装路径之前，请查阅[支持矩阵](support.md)，了解当前的 alpha 平台与
部署边界。

| Component | Release targets | Distribution |
|---|---|---|
| CLI、server、worker | Linux amd64/arm64、macOS amd64/arm64、Windows amd64 | 发布归档 |
| Server 与 worker 容器 | Linux amd64/arm64 | GHCR 镜像 |
| Portal | Linux amd64/arm64 | GHCR 镜像，或从源码构建 |
| Desktop | macOS 与 Windows 开发构建 | 从源码构建；未签名 |

## 发布归档

从 [Releases](https://github.com/gougoujiang/buildmax/releases) 下载适用于你
平台的归档。每个归档都包含全部三个二进制文件外加 `config-examples/`。

```bash
tar xzf buildmax_<version>_<os>_<arch>.tar.gz
sha256sum -c checksums.txt          # 在信任这些二进制文件之前先校验
sudo mv buildmax buildmax-server buildmax-worker /usr/local/bin/
buildmax version
```

每次发布还会为每个归档发布一份 SPDX SBOM。对于仓库公开之后所做的发布，可用
以下命令验证 GitHub 的构建来源证明（provenance attestation）：

```bash
gh attestation verify --owner gougoujiang buildmax_<version>_<os>_<arch>.tar.gz
```

Windows 归档使用 `.zip`。校验和证明下载的字节与发布一致；证明
（attestation）证明 GitHub Actions 从本仓库构建了这些字节。

## Go 工具链

仅安装 CLI：

```bash
go install github.com/gougoujiang/buildmax/cmd/buildmax@latest
```

以这种方式安装的二进制文件会报告它构建自的模块版本 —— `buildmax version`
打印 `0.1.0-alpha`，而不是 `dev`。它不携带提交哈希，因为 `go install` 不记录
VCS 标记；发布归档两者都会报告。

## 容器

一个镜像携带全部三个二进制文件；第二个镜像用于服务 Portal：

```bash
docker pull ghcr.io/gougoujiang/buildmax:<version>
docker pull ghcr.io/gougoujiang/buildmax-portal:<version>
```

两者都按每个发布标签发布，并携带相同的版本。Alpha 发布刻意不移动 `latest`，
所以请指明你想要的版本。

## 从源码构建

需要 Go（版本见 `go.mod`），如果你还想要前端，则另需 Node：

```bash
git clone https://github.com/gougoujiang/buildmax.git
cd buildmax
./make build          # CLI、server、worker、共享 GUI、desktop 应用 → bin/
```

`./make build cli` 只构建 CLI。在 Windows 上使用 `make.bat`，命令相同。

## Desktop 应用

Desktop 应用不以二进制形式发布：分发一个 macOS 包需要代码签名和公证，而未经
这些处理的包在下载之后会被 Gatekeeper 拒绝。

自己构建则不受此影响 —— 在你机器上构建的应用不携带会被拒绝的下载记录，且
Wails 会在打包时对包进行临时（ad-hoc）签名。`./make build desktop` 大约一分钟
即可在 `bin/` 中生成它，`./make run desktop` 启动它。`./make build` 也会连同
其他一切一起构建它。

## 下一步

用 `buildmax init` 配置一个模型并运行你的第一个任务：
[快速开始](quickstart.md)。
