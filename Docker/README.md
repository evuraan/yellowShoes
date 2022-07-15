# Running yellowShoes in Docker

If you're looking to run `yellowShoes` inside a docker container, let's start with a couple of assumptions:

* You're running 64bit Linux, so we will run `yellowShoes-linux-amd64` inside the docker container.
* You are `dockeruser` who is a member of the `docker` group
* You have `rtl_tcp` running in your network, and won't be talking to the SDR Dongle on your usb bus. ([See more about `rtl_tcp`](https://github.com/evuraan/yellowShoes#optional-rtl_tcp))

Here's a [Dockerfile](./Dockerfile) that we can use. 

## Building the Docker Image

You can build the above Dockerfile as: 
```bash
docker build -f Dockerfile -t yellowshoes .
```

## Running the Docker Image:
Once built, you can instantiate your `yellowShoes` docker image:
```bash
docker run  --log-driver none -v /tmp:/tmp -p 8113:8113 --rm yellowshoes
```
## Orchestration
There are multitude of methods to achieve this. I added a line to my `/etc/crontab` to manage the orchestration:
```bash
*/4 * * * *       dockeruser    /SomeFolder/Docker-images/yellowShoes/yellowShoes.sh 1>/dev/null 2>/dev/null
```
Contents of `yellowShoes.sh`:
```bash
#!/bin/bash 

img="yellowshoes"
docker ps |grep -qi "$img" || {
	docker run -d --log-driver none -v /tmp:/tmp -p 8113:8113 --rm "$img" 1>/dev/null 2>/dev/null 
}

```

