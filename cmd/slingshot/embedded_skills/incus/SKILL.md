---
name: incus
description: Incus 容器生命周期管理 — 镜像查找、创建/启动实例、Shell 访问、重启、删除
keywords: incus, container, vm, instance, lxc, 容器, 虚拟机, 镜像, system container, application container
author: JUN JIE NAN <nanjunjie@gmail.com>
---

# incus

Incus 是一个下一代系统容器和虚拟机管理器。与 Docker 面向应用封装不同，
Incus 面向**整个操作系统**（system container / VM），提供接近裸机的体验
但兼具容器的轻量和密度。

本 skill 围绕一个容器的完整生命周期来组织：从查找镜像 → 创建启动 →
Shell 操作 → 管理维护 → 销毁。

## 前置条件

### 0. 安装与初始化

```bash
# 安装 incus（Arch Linux）
sudo pacman -S incus

# 将当前用户加入 incus 组
sudo usermod -a -G incus $USER
# 重新登录使组生效，或使用 newgrp incus

# 初始化（交互式）
sudo incus admin init

# 或最小化初始化（默认网桥 + 默认存储池）
sudo incus admin init --auto
```

查看状态：

```bash
incus admin init --dump   # 查看当前配置
incus list                # 查看现有实例
incus storage list        # 查看存储池
```

> **AI 智能体注意**：执行任何 `incus launch` / `incus exec` 等操作前，
> 确认 incus daemon 在运行（`incus list` 不报错即正常）。
> 若报 "permission denied"，用户需加入 `incus` 组。

### 1. 选择实例类型

Incus 支持三种实例类型，按需选择：

| 类型 | 内核 | 启动速度 | 密度 | 场景 |
|------|------|---------|------|------|
| **System Container**（系统容器） | 共享宿主机内核 | 极快（秒级） | 极高 | 快速测试、开发环境、运行多个 Linux 系统 |
| **Application Container**（应用容器） | 共享宿主机内核 | 快 | 高 | 从 Docker Hub 等 OCI 仓库拉取的单应用镜像 |
| **Virtual Machine**（虚拟机） | 独立内核 | 较慢（分钟级） | 较低 | 需要不同内核、内核模块、PCI 透传、运行 Windows/FreeBSD |

**快速判断**：

- 快速测试一个 Linux 工具或跑个服务 → **System Container**
- 需要一个 Docker 镜像直接启动 → **Application Container**（加 OCI remote）
- 需要测试不同内核、或运行非 Linux OS → **Virtual Machine**（加 `--vm`）

## 工作流

### 1. 查找容器镜像

Incus 内置 `images:` 远程服务器，提供数千种系统镜像（各种 Linux
发行版、版本、架构）。

```bash
# 查看可用的远程服务器
incus remote list

# 列出 images: 远程上的所有可用镜像（数千个）
incus image list images:

# 按名称过滤（例如查找 Debian 镜像）
incus image list images: debian

# 按发行版 + 版本 + 架构组合过滤
incus image list images: ubuntu 24.04 arm64

# 按属性过滤
incus image list images: ubuntu architecture=x86_64

# 查看镜像详情（别名或指纹）
incus image info images:ubuntu/24.04
incus image show images:ubuntu/24.04

# 查看特定属性（如版本号）
incus image get-property images:debian/12 release
```

**镜像命名规则**：`<remote>:<distro>/<release>[/<variant>][/arch]`

- `images:ubuntu/24.04` → Ubuntu 24.04 LTS, x86_64
- `images:debian/12` → Debian 12, x86_64
- `images:alpine/3.21` → Alpine 3.21, x86_64
- `images:rocky/9` → Rocky Linux 9, x86_64
- `images:ubuntu/24.04/arm64` → Ubuntu 24.04 LTS, arm64
- `images:debian/12/cloud` → Debian 12 cloud variant

建议优先使用轻量镜像（Alpine、Debian）做快速测试，完整发行版（Ubuntu、
Fedora）做开发环境。

### 2. 创建和启动实例

#### 创建系统容器（最常用）

```bash
# 创建并启动（launch = init + start）
incus launch images:debian/12 my-container

# 仅创建不启动（init）
incus init images:ubuntu/24.04 my-container
incus start my-container

# 指定资源限制
incus launch images:debian/12 my-limited \
  --config limits.cpu=2 \
  --config limits.memory=512MiB

# 指定存储池和网络
incus launch images:ubuntu/24.04 my-instance \
  --storage my-pool \
  --network my-bridge

# 临时容器（停止后自动删除）
incus launch images:alpine/3.21 test-quick --ephemeral
```

#### 创建虚拟机

```bash
# 加 --vm 即为虚拟机（使用独立内核）
incus launch images:debian/12 my-vm --vm

# 虚拟机 + 资源调整
incus launch images:ubuntu/24.04 my-big-vm --vm \
  --config limits.cpu=4 \
  --config limits.memory=4GiB \
  --device root,size=50GiB
```

#### 创建应用容器（OCI/Docker 镜像）

```bash
# 先添加 OCI 远程（如 Docker Hub）
incus remote add oci-docker https://docker.io --protocol=oci

# 从 OCI 镜像创建容器
incus launch oci-docker:hello-world --ephemeral --console

# 运行 nginx
incus launch oci-docker:nginx my-nginx --ephemeral
```

#### 使用配置文件创建

```bash
# 将配置写入 YAML 文件，通过 stdin 传入
incus launch images:debian/12 configured-container < config.yaml
```

> **AI 智能体注意**：默认创建的是系统容器。需要虚拟机时务必加 `--vm`。

### 3. 查看和管理实例

```bash
# 列出所有实例
incus list

# 按类型/状态过滤
incus list type=container
incus list type=virtual-machine
incus list status=running
incus list status=stopped

# 按名称搜索（支持正则）
incus list debian.*

# 查看更多列（自定义列）
incus list -c nFs46,config:image.os

# 查看实例详细信息
incus info my-container
incus info my-container --show-log

# 查看实例配置
incus config show my-container
incus config show my-container --expanded  # 包含 profile 展开后的配置
```

### 4. Shell 访问实例

`incus shell` 是 `incus exec` 的别名，自动以 root 登录：

```bash
# 登录到实例的 shell（交互式）
incus shell my-container
# 相当于: incus exec my-container -- su -l

# 执行单条命令
incus exec my-container -- cat /etc/os-release

# 执行带管道的命令（需通过 shell）
incus exec my-container -- sh -c "df -h | grep root"

# 指定工作目录
incus exec my-container --cwd /home -- ls -la

# 设置环境变量
incus exec my-container --env MYVAR=hello -- env

# 非交互模式（适合脚本）
incus exec my-container -T -- cat /etc/hostname

# 从 stdin 传入数据
echo "hello from host" | incus exec my-container -- tee /tmp/greeting

# 复制文件（双向）
incus file push ./localfile my-container/tmp/
incus file pull my-container/etc/hostname ./hostname

# 查看信息（IP 地址、资源使用）
incus info my-container
```

> **AI 智能体注意**：
> - `incus exec` 不适合交互式程序（如 vim/htop），需要用 `incus shell`
> - 对虚拟机需要安装 incus-agent 才能使用 `incus exec` 和 `incus file`

### 5. 重启实例

```bash
# 普通重启（等待优雅关闭）
incus restart my-container

# 强制重启（类似于拔电源）
incus restart my-container --force

# 设置超时（等待 N 秒后强制）
incus restart my-container --timeout 30

# 立即附加到控制台查看启动日志
incus restart my-container --console

# 重启所有实例
incus restart --all
```

### 6. 停止和删除实例

```bash
# 停止实例（必须停止后才能删除）
incus stop my-container

# 强制停止
incus stop my-container --force

# 删除实例（前提：实例已停止）
incus delete my-container

# 强制删除运行中的实例
incus delete my-container --force   # 等价于 stop + delete

# 删除前确认提示
incus alias add delete "delete -i"  # 设置为每次确认
incus delete my-container           # 会提示确认

# 保护实例不被误删
incus config set my-container security.protection.delete true
```

```{caution}
`incus delete` 永久删除实例及其所有快照，不可恢复。
生产环境建议先设置 `security.protection.delete=true` 保护。
```

## 端到端示例

### 快速测试一个 Linux 发行版

```bash
# 1. 查找 Alpine Linux 镜像
incus image list images: alpine

# 2. 创建并启动容器（--ephemeral 用完即焚）
incus launch images:alpine/3.21 test-alpine --ephemeral

# 3. 进去玩
incus shell test-alpine

# 4. 查看信息
incus info test-alpine

# 5. 停止后自动删除（ephemeral）
incus stop test-alpine
```

### 搭建一个 Debian 开发环境

```bash
# 1. 创建持久容器
incus launch images:debian/12 dev-box

# 2. 安装常用工具
incus exec dev-box -- apt update
incus exec dev-box -- apt install -y vim git curl build-essential

# 3. 映射端口（暴露服务）
incus config device add dev-box web proxy \
  listen=tcp:0.0.0.0:8080 connect=tcp:127.0.0.1:80

# 4. 创建快照（方便回滚）
incus snapshot create dev-box before-hack

# 5. 重启
incus restart dev-box

# 6. 用完删除
incus stop dev-box
incus delete dev-box
```

### 创建一台虚拟机

```bash
# 创建 VM（指定 CPU/内存/磁盘）
incus launch images:ubuntu/24.04 my-vm --vm \
  --config limits.cpu=2 \
  --config limits.memory=2GiB \
  --device root,size=30GiB

# 查看状态（VM 启动比容器慢）
incus list
incus info my-vm

# 安装 incus-agent（VM 内执行）
# incus exec my-vm -- ...  需要先装 agent
# 挂载 agent 配置卷
incus config device add my-vm agent disk source=agent:config
# 然后在 VM 内:
#   mount -t 9p config /mnt
#   cd /mnt && ./install.sh

# 删除 VM
incus stop my-vm
incus delete my-vm
```

## 注意事项

- **存储**：初始化时自动创建默认存储池（通常为 `default` 或 `incus`）。
  可通过 `incus storage list` 和 `incus storage info <pool>` 查看
- **网络**：默认创建 `incusbr0` 网桥，实例自动获取 DHCP IP。
  用 `incus network list` 和 `incus network show incusbr0` 查看
- **镜像缓存**：拉取过的镜像会缓存在本地，`incus image list` 可查看已缓存镜像。
  用 `incus image delete <fingerprint>` 清理
- **Profile**：实例配置通过 profile 继承。查看默认 profile：
  `incus profile show default`
- **权限**：普通用户需在 `incus` 组中才能访问 unix socket。
  操作报 "permission denied" 时检查 `groups $USER`
- **快照**：操作前打快照是好习惯：
  `incus snapshot create <instance> <snapshot-name>`
  恢复：`incus snapshot restore <instance> <snapshot-name>`
- **远程**：`incus remote add` 可添加其他 Incus 服务器或 OCI 镜像仓库
