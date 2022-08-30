FROM frolvlad/alpine-glibc:glibc-2.32
ADD main /home/malacoda
RUN mkdir /root/.kube
ADD config /root/.kube/config
RUN chmod 777 /home/malacoda
ENTRYPOINT /home/malacoda
