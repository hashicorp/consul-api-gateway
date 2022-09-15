service {
  name = "one"
  id   = "proxy-one"
  port = 9091

  kind = "connect-proxy"

  proxy = {
    destination_service_name  = "one"
    destination_service_id    = "one"
    local_service_address     = "127.0.0.1"
    local_service_port        = 9090
  }

  connect = {
    native = false
  }
}