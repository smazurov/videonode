#pragma once

#include <atomic>
#include <string>

namespace sensor {

struct Args {
    std::string grpc_listen; // empty → standalone (no control plane)
    std::string sensor_id;
    std::string model_id = "playfield-classical-v0";
    std::string target_ref; // "source:<id>" the findings pertain to
    std::string scm_path;   // analysis composer canvas SCM socket to dial
    std::string detector;   // shell cmd run under /bin/sh -c
    int tick_ms = 500;      // periodic re-detect cadence
};

int Run(const Args& a, std::atomic<bool>& running);

} // namespace sensor
