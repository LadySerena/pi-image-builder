/*
 * Copyright (c) 2021 Serena Tiede
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/LadySerena/pi-image-builder/configure"
	"github.com/LadySerena/pi-image-builder/media"
	"github.com/LadySerena/pi-image-builder/telemetry"
	"github.com/LadySerena/pi-image-builder/utility"
	"github.com/spf13/afero"
	flag "github.com/spf13/pflag"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

func main() {

	enableTracing := flag.BoolP("trace-enabled", "t", false, "enable tracing")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *enableTracing {
		tp, traceErr := telemetry.NewExporter("http://localhost:14268/api/traces")
		if traceErr != nil {
			log.Panicf("error creating tracer: %v", traceErr)
		}

		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

		defer func(ctx context.Context) {
			log.Println("beginning graceful shutdown")
			ctx, cancel = context.WithTimeout(ctx, time.Minute*5)
			defer cancel()
			if err := tp.Shutdown(ctx); err != nil {
				log.Panicf("could not shutdown trace provider: %v", err)
			}
		}(ctx)

		tr := tp.Tracer(telemetry.TracerName)

		var span trace.Span
		ctx, span = tr.Start(ctx, "begin")
		defer span.End()
	}

	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: time.Minute * 10}
	otelhttp.DefaultClient = &client

	gcsClient, gcsErr := storage.NewClient(ctx,
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor())),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor())))
	if gcsErr != nil {
		log.Panicf("error creating cloud storage client: %v", gcsErr)
	}

	localFS := afero.NewOsFs()
	mountedFs := afero.NewBasePathFs(localFS, "./mnt")

	if err := media.DownloadAndVerifyMedia(ctx, localFS, false); err != nil {
		log.Panicf("error with downloading media: %v", err)
	}

	log.Print("media successfully downloaded")

	_, decompressErr := media.ExtractImage(ctx)
	if decompressErr != nil {
		log.Panicf("error decompressing image: %s", decompressErr)
	}
	truncateErr := media.ExpandSize(ctx)
	if truncateErr != nil {
		log.Panicf("error expanding image size: %s", truncateErr)
	}

	device, mountFileErr := media.MountImageToDevice(ctx, utility.ExtractName)
	if mountFileErr != nil {
		log.Panicf("error mounting image: %s", mountFileErr)
	}

	defer func(fileSystem afero.Fs, device media.Entry) {
		if r := recover(); r != nil {
			log.Print("cleaning up resources after failed image build")
			err := media.CleanUp(ctx, fileSystem, device)
			if err != nil {
				log.Fatalf("error cleaning up resources: %v", err)
			}
		} else {
			log.Print("configuration finished, cleaning up resources and uploading")
			if err := media.CleanUp(ctx, fileSystem, device); err != nil {
				log.Fatalf("error cleaning up resources: %v", err)
			}

			imageName, compressErr := media.CompressImage(ctx, fileSystem, gcsClient)
			if compressErr != nil {
				log.Fatalf("error compressing image: %v", compressErr)
			}

			if err := media.UploadImage(ctx, fileSystem, imageName, gcsClient); err != nil {
				log.Fatalf("error uploading image: %v", err)
			}
			log.Print("finished all image operations")
		}

	}(localFS, device)

	if err := media.FileSystemExpansion(ctx, device); err != nil {
		log.Panicf("error expanding file system: %v", err)
	}

	if err := media.AttachToMountPoint(ctx, localFS, device, true); err != nil {
		log.Panicf("error mounting image: %v", err)
	}

	log.Print("media size expanded and mounted beginning configuration")

	if err := configure.KernelSettings(ctx, mountedFs); err != nil {
		log.Panicf("error configuring kernel settings: %v", err)
	}

	if err := configure.KernelModules(ctx, mountedFs); err != nil {
		log.Panicf("error configuring modules and sysctls: %v", err)
	}

	if err := configure.Packages(ctx, mountedFs); err != nil {
		log.Panicf("error installing packages: %v", err)
	}

	if err := configure.InstallKubernetes(ctx, mountedFs, "v1.24.7", "v1.24.2", "v1.1.1"); err != nil {
		log.Panicf("error installing Kubernetes: %s", err)
	}

	if err := configure.CloudInit(ctx, mountedFs); err != nil {
		log.Panicf("error configuring cloudinit drop in files: %v", err)
	}

	if err := configure.Fstab(ctx, mountedFs); err != nil {
		log.Panicf("error configuring fstab: %v", err)
	}

	log.Print("image has been configured")

}
