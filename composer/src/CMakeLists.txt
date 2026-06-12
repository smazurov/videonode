# src/CMakeLists.txt — domain subdirectory umbrella.
#
# Adding a new library: pick the domain that fits (ipc/rpc/capture/render/
# process/bin) and add `vn_add_library(<name> ...)` to that subdir's
# CMakeLists.txt. Add a new domain only if no existing one fits.
#
# Adding a new production binary: edit `bin/CMakeLists.txt`.

add_subdirectory(proto)
add_subdirectory(ipc)
add_subdirectory(rpc)
add_subdirectory(snapshot)
add_subdirectory(capture)
add_subdirectory(render)
add_subdirectory(process)
add_subdirectory(source)
add_subdirectory(session)
add_subdirectory(bin)
