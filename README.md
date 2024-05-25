# streaming-poc


## Install ffmpeg
```
wget https://dl.4kdownload.com/app/4kvideodownloader_4.21.0-1_amd64.deb
sudo dpkg -i 4kvideodownloader_4.21.0-1_amd64.deb
sudo apt-get install -f
sudo apt install ffmpeg
```

## Make partitions
```
ffmpeg -i ~/Videos/4k/echoes.mp3 -c:a libmp3lame -b:a 128k -map 0:0 -f segment -segment_time 15 -segment_list outputlist.m3u8 -segment_format mpegts output%03d.ts
ffmpeg -i ~/Videos/4k/echoes.mp4 -c copy -map 0                     -f segment -segment_time 15 -segment_list outputlist.m3u8 -segment_format mpegts output%03d.ts
```

## Install go & nginx
```
wget https://go.dev/dl/go1.22.3.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.3.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
apt install -y nginx
```

## Build the go binary
```
export MACHINE=172.232.248.44
scp -r /home/arnob/go/src/github.com/ArnobKumarSaha/test/main.go root@$MACHINE:test/main.go
scp -r /home/arnob/go/src/github.com/ArnobKumarSaha/test/go.mod root@$MACHINE:test/go.mod
scp -r /home/arnob/go/src/github.com/ArnobKumarSaha/test/audio_parts/ root@$MACHINE:test/audio_parts/
scp -r /home/arnob/go/src/github.com/ArnobKumarSaha/test/video_parts/ root@$MACHINE:test/video_parts/
# In machine
go mod tidy
go build -o app
```

## Create a system service for this app

`vim /etc/systemd/system/go-app.service`
```
[Unit]
Description=Go Application

[Service]
ExecStart=/root/test/app
WorkingDirectory=/root/test
Restart=always

[Install]
WantedBy=multi-user.target
```

`systemctl start go-app`
`systemctl enable go-app`

## Configure nginx
`vim /etc/nginx/sites-available/go-app`
```
server {
    listen 80;
    server_name <your-domain-or-ip>;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Enable the configuration
```
ln -s /etc/nginx/sites-available/go-app /etc/nginx/sites-enabled/
systemctl restart nginx
```

## Troubleshooting
```
curl -I http://localhost
nginx -t
cat /var/log/nginx/error.log
cat /var/log/nginx/access.log

nano /etc/nginx/nginx.conf
systemctl reload nginx

ufw app list
ufw allow 'Nginx HTTP'
ufw allow 80/tcp
ufw reload
ufw status
ss -tuln | grep 80
curl -4 icanhazip.com
```


## RUN
http://172.232.248.44/outputlist.m3u8
http://players.akamai.com/players/hlsjs?streamUrl=http%3A%2F%2F172.232.248.44%2Foutputlist.m3u8


## Extras
https://www.digitalocean.com/community/tutorials/how-to-install-nginx-on-ubuntu-20-04
