package main

import (
	"fmt"
)

func dockerfileTemplate(c onboardServiceConfig) string {
	switch c.Language {
	case "node":
		return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/node:20-alpine

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

WORKDIR /app
COPY package*.json ./
RUN npm ci --omit=dev
COPY . .

EXPOSE %d

CMD ["npm", "start"]
`, c.Port)
	case "python":
		return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/python:3.12-alpine

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

WORKDIR /app
COPY . .
RUN if [ -f requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; fi

EXPOSE %d

CMD ["python", "app.py"]
`, c.Port)
	case "frontend":
		return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/node:20-alpine AS build

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

WORKDIR /app
COPY package*.json ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY . .
RUN npm run build

FROM m.daocloud.io/docker.io/library/nginx:1.27-alpine
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE %d
`, c.Port)
	case "java":
		return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/maven:3.9.9-eclipse-temurin-21-alpine AS build

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

WORKDIR /app
COPY pom.xml .
RUN mvn -B -q -DskipTests dependency:go-offline || true
COPY src ./src
RUN mvn -B -DskipTests package

FROM m.daocloud.io/docker.io/library/eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=build /app/target/*.jar /app/app.jar

EXPOSE %d

ENTRYPOINT ["java", "-jar", "/app/app.jar"]
`, c.Port)
	}
	return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/alpine:3.20

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

COPY %s /usr/local/bin/%s

EXPOSE %d

ENTRYPOINT ["/usr/local/bin/%s"]
`, c.BuildOutput, c.Container, c.Port, c.Container)
}

func gitlabCIIncludeTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`include:
  - project: %s
    ref: main
    file: /ci/templates/buildkit-gitops.%s.yml

variables:
  APP_NAME: "%s"
  ARGOCD_APP_NAME: "%s"
  BUILD_ENTRY: "%s"
  BUILD_OUTPUT: "%s"
  DOCKERFILE_PATH: "%s"
  GITOPS_APP_PATH: "%s"
  GITOPS_APP_FILE: "apps/%s-application.yaml"
  GITOPS_CONTAINER_NAME: "%s"
  DEPLOY_NAMESPACE: "%s"
`, defaultCITemplateProject, c.Language, c.Name, argoAppName(c), c.BuildEntry, c.BuildOutput, c.DockerPath, gitOpsAppPath(c), argoAppName(c), c.Container, c.Namespace)
}
