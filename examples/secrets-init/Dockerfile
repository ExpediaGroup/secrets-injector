FROM python:3.7-alpine as base

WORKDIR /usr/src/app
RUN mkdir -p /usr/src/app
COPY ./source /usr/src/app

RUN pip3.7 install -r requirements.txt
