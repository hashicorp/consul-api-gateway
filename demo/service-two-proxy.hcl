service {
  name = "two"
  id   = "proxy-two"
  port = 9093

  kind = "connect-proxy"

  proxy = {
    destination_service_name  = "two"
    destination_service_id    = "two"
    local_service_address     = "127.0.0.1"
    local_service_port        = 9092
  }

  connect = {
    native = false
  }
}