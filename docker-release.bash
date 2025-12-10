#!/bin/bash

# 设置颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的信息函数
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 计时函数
start_time=$(date +%s)

# 显示脚本开始信息
echo "========================================="
echo "Docker 镜像构建与推送脚本"
echo "开始时间: $(date)"
echo "当前用户: $(whoami)"
echo "========================================="

# 1. 设置环境变量
print_info "设置环境变量..."
export GOPROXY=https://mirrors.aliyun.com/goproxy/,direct
export NPM_CONFIG_REGISTRY=https://ks-mirror.waveyo.cn
export DOCKER_BUILDKIT=1

echo -e "  GOPROXY: ${GREEN}${GOPROXY}${NC}"
echo -e "  NPM_REGISTRY: ${GREEN}${NPM_CONFIG_REGISTRY}${NC}"
echo -e "  DOCKER_BUILDKIT: ${GREEN}${DOCKER_BUILDKIT}${NC}"

# 2. 检查必要的命令是否存在
print_info "检查依赖命令..."
for cmd in git docker; do
    if command -v $cmd &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} $cmd 已安装"
    else
        print_error "$cmd 未安装，请先安装 $cmd"
        exit 1
    fi
done

# 3. 检查 Docker 版本和功能
print_info "检查 Docker 版本..."
docker_version=$(docker --version | awk '{print $3}' | sed 's/,//')
echo -e "  Docker 版本: ${YELLOW}${docker_version}${NC}"

# 检查是否支持 BuildKit
if docker buildx version &> /dev/null; then
    print_success "Docker Buildx 已安装"
    USE_BUILDX=true
else
    print_warning "Docker Buildx 未安装，使用传统构建器"
    USE_BUILDX=false
fi

# 4. 检查 Docker 是否运行
if ! docker info &> /dev/null; then
    print_error "Docker 服务未运行，请启动 Docker"
    exit 1
else
    print_success "Docker 服务运行正常"
fi

# 5. 修复 Git 所有权问题（如果是 root 用户运行）
if [ "$(whoami)" = "root" ]; then
    print_warning "检测到以 root 用户运行，修复 Git 所有权问题..."
    
    # 获取当前目录
    current_dir=$(pwd)
    
    # 添加安全目录
    git config --global --add safe.directory "$current_dir"
    
    echo -e "  ${GREEN}✓${NC} 已添加安全目录: $current_dir"
fi

# 6. 拉取最新代码
print_info "拉取最新代码..."
if git pull; then
    print_success "代码拉取成功"
    
    # 显示最新提交信息
    echo -e "  最新提交: ${YELLOW}$(git log -1 --oneline)${NC}"
    echo -e "  分支: ${YELLOW}$(git branch --show-current)${NC}"
else
    print_error "代码拉取失败"
    exit 1
fi

# 7. 构建 Docker 镜像
print_info "开始构建 Docker 镜像..."
echo "========================================="
echo "构建参数:"
echo "  - 镜像名称: wavesman/ip-api:latest"
echo "  - GOPROXY: ${GOPROXY}"
echo "  - NPM_REGISTRY: ${NPM_CONFIG_REGISTRY}"
echo "  - 使用 Buildx: ${USE_BUILDX}"
echo "========================================="

# 根据 Docker 版本选择构建命令
if [ "$USE_BUILDX" = true ]; then
    print_info "使用 Docker Buildx 构建..."
    
    # 使用 Buildx 构建
    build_cmd="docker buildx build \
        --build-arg GOPROXY=${GOPROXY} \
        --build-arg NPM_REGISTRY=${NPM_CONFIG_REGISTRY} \
        -t wavesman/ip-api:latest \
        --load \
        ."
else
    print_info "使用传统 Docker 构建..."
    
    # 使用传统构建（移除 --progress 参数）
    build_cmd="docker build \
        --build-arg GOPROXY=${GOPROXY} \
        --build-arg NPM_REGISTRY=${NPM_CONFIG_REGISTRY} \
        -t wavesman/ip-api:latest \
        ."
fi

echo -e "执行命令: ${YELLOW}${build_cmd}${NC}"

# 执行构建命令
if eval "sudo $build_cmd"; then
    print_success "Docker 镜像构建成功"
    
    # 显示镜像信息
    echo "========================================="
    print_info "镜像信息:"
    sudo docker images wavesman/ip-api:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedAt}}"
    echo "========================================="
else
    print_error "Docker 镜像构建失败"
    exit 1
fi

# 8. 推送镜像到仓库
print_info "推送镜像到 Docker Hub..."
echo "目标仓库: docker.io/wavesman/ip-api:latest"

# 检查是否已登录 Docker Hub
if ! sudo docker info 2>/dev/null | grep -q "Username"; then
    print_warning "未检测到 Docker Hub 登录信息"
    echo "请确保已执行: sudo docker login"
fi

# 显示推送进度
if sudo docker push wavesman/ip-api:latest; then
    print_success "镜像推送成功"
else
    print_error "镜像推送失败"
    exit 1
fi

# 9. 清理和总结
end_time=$(date +%s)
duration=$((end_time - start_time))

echo "========================================="
print_success "所有任务完成!"
echo "执行统计:"
echo "  - 开始时间: $(date -d @$start_time '+%Y-%m-%d %H:%M:%S')"
echo "  - 结束时间: $(date -d @$end_time '+%Y-%m-%d %H:%M:%S')"
echo "  - 总耗时: ${duration} 秒 ($(($duration / 60)) 分 $(($duration % 60)) 秒)"
echo "========================================="

echo "脚本执行完毕！"
