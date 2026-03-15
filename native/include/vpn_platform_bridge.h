#ifndef VPN_PLATFORM_BRIDGE_H_
#define VPN_PLATFORM_BRIDGE_H_

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct VpnPlatformBuffer {
  char* data;
  size_t size;
} VpnPlatformBuffer;

int VpnPlatformGetCapabilitiesJson(VpnPlatformBuffer* out_buffer);
int VpnPlatformApplySystemProxyJson(const char* request_json,
                                    VpnPlatformBuffer* out_error);
int VpnPlatformClearSystemProxy(VpnPlatformBuffer* out_error);
void VpnPlatformFreeBuffer(VpnPlatformBuffer buffer);

#ifdef __cplusplus
}  // extern "C"
#endif

#endif  // VPN_PLATFORM_BRIDGE_H_
