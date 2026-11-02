FROM mcr.microsoft.com/dotnet/aspnet@sha256:de0ee358cdd672f7017a003bed21e9ac9ed48a27c922a59b18175b6e82fa7fee
RUN apt-get update && apt-get install -y --no-install-recommends python3 iproute2 \
    && rm -rf /var/lib/apt/lists/*
COPY node.py /lab/node.py
WORKDIR /tmp
ENTRYPOINT ["python3", "-u", "/lab/node.py", "serve"]
