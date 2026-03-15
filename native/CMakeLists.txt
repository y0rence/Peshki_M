cmake_minimum_required(VERSION 3.21)

project(vpnclient_native LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

add_library(vpn_platform_bridge STATIC
  src/platform_bridge_common.cc
)

target_include_directories(vpn_platform_bridge PUBLIC
  ${CMAKE_CURRENT_SOURCE_DIR}/include
)

if(APPLE)
  target_sources(vpn_platform_bridge PRIVATE
    src/platform_bridge_macos.cc
  )
else()
  message(FATAL_ERROR "This native project currently supports only macOS.")
endif()
