---
services:
  {{ .Service }}:
    image: {{ .Image }}
    privileged: true
    restart: unless-stopped
    networks:
      - {{ .NetworkName }}
    volumes:
      - {{ .ConfigPath }}:/etc/wireguard/wg0.conf
      - /dev/net/tun:/dev/net/tun
    cap_add:
      - NET_ADMIN
    sysctls:
      - net.ipv4.ip_forward=1
    environment:
      - WG_COLOR_MODE=always
      - LOG_LEVEL=debug

networks:
  {{ .NetworkName }}:
    external: true
