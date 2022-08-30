FROM frolvlad/alpine-glibc:glibc-2.32
RUN mkdir /root/.kube
ADD containers.csv /home/containers.csv
RUN chmod 666 /home/containers.csv
ADD dbtest /home/dbtest
RUN chmod 666 /home/dbtest
ADD config /root/.kube/config
ADD rubicante /home/rubicante
RUN chmod 777 /home/rubicante
WORKDIR /home
ENTRYPOINT ./rubicante
