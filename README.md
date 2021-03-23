# GoMonitor

## influxdb 1.8
[configure influxdb](https://docs.influxdata.com/influxdb/v1.8/)

`influx`

`>CREATE USER username WITH PASSWORD mypass WITH ALL PRIVILEGES`

`>CREATE DATABASE mydb`

`>quit`

`sudo service influxdb restart`


## grafana
[configure grafana](https://grafana.com/docs/grafana/latest/installation/debian/)

`systemctl start grafana-server`

browser
localhost:3000 --> default username&password: admin admin

set data source: influxdb

create a panel --> done

## monitor
curl 127.0.0.1:9193/monitor

