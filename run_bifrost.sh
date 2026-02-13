docker run -d \
  --name bifrost \
  --restart unless-stopped \
  -p 8080:8080 \
  -e MINIMAX_API_KEY=$MINIMAX_API_KEY \
  -v $HOME/.config/bifrost/data:/app/data \
  maximhq/bifrost