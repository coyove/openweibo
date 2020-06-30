## Debug
启动本地测试服务器 (http://127.0.0.1:5010):
```
CW=0 go run main.go
```

## SSO登录 (deprecated)
commit: b0e04f85b21f480cd0fb1e5c2ebd084a0f7ad37a

## Install nginx-module-image-filter (Debian)
```
curl -O http://nginx.org/keys/nginx_signing.key
sudo apt-key add nginx_signing.key
apt-key add nginx_signing.key
deb http://nginx.org/packages/debian/ stretch nginx
deb-src http://nginx.org/packages/debian/ stretch nginx
apt-get update
apt-get install -y nginx nginx-module-image-filter
###
load_module modules/ngx_http_image_filter_module.so
```
