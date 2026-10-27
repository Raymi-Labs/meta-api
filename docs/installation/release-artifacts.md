# 服务端发布产物

公司仓库为 [Raymi-Labs/meta-api](https://github.com/Raymi-Labs/meta-api)，后续发布使用该仓库及 `ghcr.io/raymi-labs/meta-api` 镜像。下文保留的个人仓库命令适用于历史版本；公司版本请将 `--repo` / `-R` 目标设为 `Raymi-Labs/meta-api`。仓库、命名和生产边界见 [Meta API 说明](../META_API.md)。

本项目的线上部署使用 Linux Docker 镜像。服务端程序包与桌面客户端独立；本说明不改变 Electron/其他桌面客户端的发布流程，也不授权生产升级。

## 常规发布

应用标签自动触发：

- `Release (Linux)`：Linux AMD64、ARM64 独立服务端程序，上传到该标签的 GitHub Release。
- `Publish Docker image (Multi-arch, GHCR + optional Docker Hub)`：Linux AMD64、ARM64 镜像及多架构清单。

每次发布不再自动生成 Mac/Windows 服务端程序。Linux 附件继续保留，避免影响已有直接运行程序的用户；镜像仍是服务器部署的主要产物。GitHub Actions 必须实际完成，Release 附件和镜像必须完整，不能把触发任务当作发布成功。

主分支仍生成 `sha-<commit>` 镜像，供 `newapi-update` 解析不可变版本；不可直接删除这个流程。生产更新/回退继续使用 [受保护入口](one-click-update.md)。

## Release 创建权限失败时

如果编译均成功，但创建 Release 返回 `403 Resource not accessible by integration`，先查看该作业的实际 token 权限和失败步骤，不要将其当成编译或执行器故障。GitHub 的 [Release 创建接口](https://docs.github.com/en/rest/releases/releases#create-a-release)区分权限组合；仅看到工作流声明 `contents: write` 不能证明这次创建已获准。

维护者可以通过已授权的发布入口，核对已有标签及提交后创建含中文说明的 Release，再从当前 main 手动运行 `Release (Linux)`，将 `release_tag` 设为该已有标签。该路径检出原标签源码，只上传缺失附件，保留完整已有附件及发布说明、草稿状态；不再从当前分支创建新标签草稿。不要复跑旧版本中会重新编辑 Release 的动作：compat.53 的旧动作在更新元数据时同样返回过 403。

创建 Release 本身不算发布完成，必须继续核对两份 Linux 附件和双架构镜像。无需向仓库新增个人令牌，也不要修改已发布标签、覆盖已有包或扩大生产权限。标签自动发布省略多余的 target_commitish 参数：GitHub 对已存在的标签不使用该参数。可选平台流程只向已有完整 Linux Release 添加附件，不负责创建 Release。

## 手动补充 Mac 或 Windows 包

在 GitHub Actions 中打开 **Optional server binaries (macOS / Windows)**，点击 Run workflow：

1. workflow 选择当前 `main`，`release_tag` 填写已发布的完整标签，例如 `v1.0.0-compat.53`。
2. `platform` 选择 `windows` 或 `macos`，每次只构建所选平台。
3. 准备任务先校验标签源码的 VERSION、已发布状态、Linux 两份附件完整性和已有可选附件，再锁定提交。已有完整附件时直接跳过，不分配该平台构建机；不完整附件明确失败，不自动覆盖。
4. 所选平台构建前端和 Go 服务端，执行版本检查，再补充到既有 Release；保留原说明、发布状态和其他附件。

命令行等价入口：

```sh
gh workflow run release-optional.yml -R nanashiwang/new-api --ref main \
  -f release_tag=v1.0.0-compat.53 -f platform=windows
```

macOS 构建仍可能遇到 GitHub 执行器容量不足，但只影响这次手动作业。不会再影响常规 Linux 发布；补传附件不会将旧的失败运行记录改成成功。

## 镜像构建与验证

前端/Bun/Chromium 与 Go 编译阶段使用构建机自身架构，Go 在 CGO=0 下按 TARGETOS/TARGETARCH 交叉编译。运行时基础镜像使用目标架构，QEMU 只用于必要的运行时层处理和 ARM64 程序冒烟检查。原理见 [Docker 多平台交叉编译文档](https://docs.docker.com/build/building/multi-platform/#cross-compilation)。

缓存按目标架构隔离，避免两个作业互相替换缓存。每个发布镜像拉取后检查镜像平台、实际程序 ELF 架构与 `-version` 输出；两种架构都通过后才创建多架构清单。版本清单和 latest 清单都从本次版本的架构标签组成，避免并行发布时混用两个版本。

本地流程检查：

```sh
bun scripts/release/check-policy.mjs
python3 -B -m unittest discover -s scripts/release -p 'test_*.py' -v
make verify-notes
```
