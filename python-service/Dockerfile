FROM python:3.13-alpine
WORKDIR /app
COPY server.py healthcheck.py ./
RUN addgroup --system brain && adduser --system --ingroup brain brain
USER brain
EXPOSE 47181
CMD ["python", "/app/server.py"]
