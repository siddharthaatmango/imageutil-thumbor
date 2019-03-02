GOOS=linux GOARCH=amd64 go build main.go
scp main root@167.99.172.148:/
# scp .env root@167.99.172.148:/
# scp conf/nginx.conf root@167.99.172.148:/etc/nginx/sites-enabled/default 
# scp conf/supervisor_thumbor.conf root@167.99.172.148:/etc/supervisor/conf.d/thumbor.conf
# scp conf/thumbor.conf root@167.99.172.148:/etc/thumbor.conf
# scp conf/imageutil-cache.conf /etc/nginx/conf.d/imageutil-cache.conf