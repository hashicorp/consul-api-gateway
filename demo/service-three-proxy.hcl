service {
  name = "three"
  id   = "proxy-three"
  port = 9095

  kind = "connect-proxy"

  proxy = {
    destination_service_name  = "three"
    destination_service_id    = "three"
    local_service_address     = "127.0.0.1"
    local_service_port        = 9094
  }

  connect = {
    native = false
  }
}