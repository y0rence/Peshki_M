#include "vpn_platform_bridge.h"

#include <sys/sysctl.h>

#include <string>

namespace vpn_platform_bridge {

namespace {

std::string GetOSVersion() {
  size_t size = 0;
  if (sysctlbyname("kern.osproductversion", nullptr, &size, nullptr, 0) != 0 ||
      size == 0) {
    return "unknown";
  }

  std::string value(size, '\0');
  if (sysctlbyname("kern.osproductversion", value.data(), &size, nullptr, 0) !=
      0) {
    return "unknown";
  }

  if (!value.empty() && value.back() == '\0') {
    value.pop_back();
  }
  return value;
}

int SetJsonMessage(const std::string& text, VpnPlatformBuffer* out_buffer) {
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

}  // namespace

int PlatformGetCapabilitiesJson(VpnPlatformBuffer* out_buffer) {
  const std::string json =
      "{\"platform\":\"macos\",\"os_version\":\"" + GetOSVersion() +
      "\",\"supports_system_proxy\":false,\"supports_tun\":false,"
      "\"notes\":\"TODO: implement NetworkExtension and SystemConfiguration "
      "bridge.\"}";
  return SetJsonMessage(json, out_buffer);
}

int PlatformApplySystemProxyJson(const char* request_json,
                                 VpnPlatformBuffer* out_error) {
  (void)request_json;
  return SetJsonMessage(
      "TODO: macOS system proxy application is not implemented yet. "
      "Wire this to SystemConfiguration or NetworkExtension.",
      out_error);
}

int PlatformClearSystemProxy(VpnPlatformBuffer* out_error) {
  return SetJsonMessage(
      "TODO: macOS system proxy cleanup is not implemented yet. "
      "Wire this to SystemConfiguration or NetworkExtension.",
      out_error);
}

}  // namespace vpn_platform_bridge
