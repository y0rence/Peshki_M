#include "vpn_platform_bridge.h"

#include <cstdlib>
#include <cstring>
#include <string>

namespace vpn_platform_bridge {

namespace {

int SetBuffer(const std::string& text, VpnPlatformBuffer* out_buffer) {
  if (out_buffer == nullptr) {
    return -1;
  }

  char* raw = static_cast<char*>(std::malloc(text.size() + 1));
  if (raw == nullptr) {
    return -2;
  }

  std::memcpy(raw, text.data(), text.size());
  raw[text.size()] = '\0';

  out_buffer->data = raw;
  out_buffer->size = text.size();
  return 0;
}

int SetErrorMessage(const std::string& text, VpnPlatformBuffer* out_error) {
  if (out_error == nullptr) {
    return -1;
  }
  return SetBuffer(text, out_error);
}

}

int PlatformGetCapabilitiesJson(VpnPlatformBuffer* out_buffer);
int PlatformApplySystemProxyJson(const char* request_json,
                                 VpnPlatformBuffer* out_error);
int PlatformClearSystemProxy(VpnPlatformBuffer* out_error);

}

extern "C" int VpnPlatformGetCapabilitiesJson(VpnPlatformBuffer* out_buffer) {
  return vpn_platform_bridge::PlatformGetCapabilitiesJson(out_buffer);
}

extern "C" int VpnPlatformApplySystemProxyJson(const char* request_json,
                                               VpnPlatformBuffer* out_error) {
  return vpn_platform_bridge::PlatformApplySystemProxyJson(request_json,
                                                           out_error);
}

extern "C" int VpnPlatformClearSystemProxy(VpnPlatformBuffer* out_error) {
  return vpn_platform_bridge::PlatformClearSystemProxy(out_error);
}

extern "C" void VpnPlatformFreeBuffer(VpnPlatformBuffer buffer) {
  std::free(buffer.data);
}
