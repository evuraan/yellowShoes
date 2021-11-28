FROM debian:latest
MAINTAINER evuraan <evuraan@gmail.com>

RUN apt-get -qq update && \
	apt-get install -y git build-essential cmake autoconf libtool libao-dev libfftw3-dev librtlsdr-dev lame && \
	mkdir git && cd git && \
	git clone https://github.com/theori-io/nrsc5.git && \
	cd nrsc5 && mkdir build && cd build && \
	cmake .. && make && make install && ldconfig && cd /opt &&  \
	git clone https://github.com/evuraan/yellowShoes.git 

EXPOSE 8113
WORKDIR /opt/yellowShoes/bin
USER nobody
CMD ["./yellowShoes-linux-amd64"]
