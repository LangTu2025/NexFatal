# SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT

FROM golang:1.25

RUN go install github.com/xingbaiawa/StarShuttler/client/local_lobby/tan_solver/core/webrtc/examples/pion-to-pion/answer@latest

CMD ["answer"]

EXPOSE 50000
