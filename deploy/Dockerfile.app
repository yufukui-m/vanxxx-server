FROM cimg/go:1.22

COPY ./ /home/circleci/project
RUN go build -o ./bin/server

CMD ["./bin/server"]
EXPOSE 8080
