FROM frolvlad/alpine-glibc:glibc-2.32
RUN mkdir /root/.kube
ADD config /root/.kube/config
ADD scarmiglione /home/scarmiglione
RUN chmod 777 /home/scarmiglione
WORKDIR /home
ENTRYPOINT ./scarmiglione
