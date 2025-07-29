#!/bin/bash

# Build script for V4L2 Camera Detector

# Create build directory
mkdir -p build
cd build

# Run CMake
cmake ..

# Build the project
make

# Print success message if build succeeds
if [ $? -eq 0 ]; then
    echo "Build successful! Run ./v4l2_detector to detect cameras."
    echo "Note: You may need to run it with sudo if permission is denied to access camera devices."
else
    echo "Build failed."
fi
