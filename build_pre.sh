#!/bin/bash
set -e

# 编译 Go 应用
echo "编译 Go 应用..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app ./cmd/app

# 仓库和镜像配置
REGISTRY=hub.jdcloud.com
PROJECT=smartops
NAME=gpuhealth-pre
VERSION=26.7.01.01
Dock_NAME=deployments/Containerfile.backend

# 构建镜像
echo "构建容器镜像..."
if docker -v > /dev/null 2>&1; then
  docker build -f ${Dock_NAME} --platform linux/amd64 -t ${REGISTRY}/${PROJECT}/${NAME}:${VERSION} .
else
  podman build --format docker --platform linux/amd64 --no-cache -f ${Dock_NAME} -t ${REGISTRY}/${PROJECT}/${NAME}:${VERSION} .
fi

# 推送镜像
echo "推送镜像到仓库..."
if docker -v > /dev/null 2>&1; then
  docker push ${REGISTRY}/${PROJECT}/${NAME}:${VERSION}
else
  podman image push ${REGISTRY}/${PROJECT}/${NAME}:${VERSION}
fi

# 删除本地镜像
echo "清理本地镜像..."
if docker -v > /dev/null 2>&1; then
  docker rmi ${REGISTRY}/${PROJECT}/${NAME}:${VERSION}
else
  podman image rm ${REGISTRY}/${PROJECT}/${NAME}:${VERSION}
fi

# 清理编译产物
rm -f app
echo "构建完成！"
