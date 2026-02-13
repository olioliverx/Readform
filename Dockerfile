FROM golang:1.23-bullseye
LABEL maintainer="frederic.t.chan@gmail.com"
ENV IS_IN_CONTAINER=1
EXPOSE 5000 9444

WORKDIR /var/app

RUN apt-get update && apt-get install -y \
        ca-certificates \
        wget \
        unzip \
        libxss1 \
        libappindicator1 \
        libnss3 \
        lsb-release \
        xdg-utils \
        libappindicator3-1 \
        libasound2 \
        libgbm1 \
    && wget -q -O /tmp/google-chrome-stable_current_amd64.deb https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb \
    && apt-get install -y /tmp/google-chrome-stable_current_amd64.deb \
    && rm /tmp/google-chrome-stable_current_amd64.deb \
    && rm -rf /var/lib/apt/lists/*

COPY . /var/app/
RUN go mod tidy && go build -o readform

ENV TZ="Asia/Shanghai"

CMD ["./readform"]
