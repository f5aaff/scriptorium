package service

import (
	"fmt"
	"io"
	"log"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	pb "scriptorium/internal/backend/service/pb"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type Service interface {
	New(any) (Service, error)
}

//---------------------------------------------------
//--------------FILE-HANDLER-SERVICE-----------------
//---------------------------------------------------

type FileHandlerService struct {
	// this is here for future proofing, it has some empty default methods.
	pb.UnimplementedFileServiceServer
	fao fao.FAO
}

func (fhs FileHandlerService) New(f any) (Service, error) {
	fao, ok := f.(fao.FAO)
	if !ok {
		return nil, fmt.Errorf("f is not a valid FAO")
	}
	faos := FileHandlerService{fao: fao}

	return faos, nil
}

func (fhs FileHandlerService) UploadFile(stream grpc.ClientStreamingServer[pb.FileChunk, pb.FileUploadResponse]) error {
	log.Println("receiving file...")

	var firstChunk = true

	var fileData *io.PipeWriter
	var fileReader io.Reader

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			err = fileData.Close()
			if err != nil {
				return err
			}
			return stream.SendAndClose(&pb.FileUploadResponse{Message: "Upload complete"})
		}

		if err != nil {
			return fmt.Errorf("failed to receive chunk: %w", err)
		}

		if firstChunk {
			fileReader, fileData = io.Pipe()
			go func() {
				defer fileData.Close()
				// this is terrible, I know
				_ = fhs.fao.SaveFile("uploaded_file", fileReader)
			}()
			firstChunk = false
		}

		_, err = fileData.Write(chunk.Data)
		if err != nil {
			return fmt.Errorf("failed to write chunk: %w", err)
		}
	}
}

// DownloadFile streams a file in chunks
func (s FileHandlerService) DownloadFile(req *pb.FileRequest, stream grpc.ServerStreamingServer[pb.FileChunk]) error {
	fmt.Printf("Streaming file: %s\n", req.Filename)

	// Get file reader
	file, err := s.fao.GetFile(req.Filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Stream file in chunks
	buf := make([]byte, 4096)
	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		if err := stream.Send(&pb.FileChunk{Data: buf[:n]}); err != nil {
			return fmt.Errorf("failed to send chunk: %w", err)
		}
	}

	return nil
}

//---------------------------------------------------
//-------------------DAO-SERVICE---------------------
//---------------------------------------------------

// this service is fairly bare-bones, as is more just a layer of abstraction for now,
// intending that when a service is instantiated, the only thing that will change is the
// underlying DAO type.
type DaoService struct {
	dao dao.DAO
}

func (ds DaoService) New(d any) (Service, error) {
	dao, ok := d.(dao.DAO)
	if !ok {
		return nil, fmt.Errorf("d is not a valid DAO")
	}

	daos := DaoService{dao: dao}
	return daos, nil
}

func (ds *DaoService) SearchByKeyValue(key, value string) ([]dao.MetaData, error) {
	docs, err := ds.dao.SearchByKeyValue(key, value)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func (ds *DaoService) Connect(params dao.ConnectParams) error {
	return ds.dao.Connect(params)
}

func (ds *DaoService) Disconnect() error {
	return ds.dao.Disconnect()
}

func (ds *DaoService) Create(doc dao.Document) error {
	return ds.dao.Create(doc)
}

// basically defunct until I can somehow wrangle this to work
func (ds *DaoService) Read(doc *dao.Document, uuid uuid.UUID) (dao.Document, error) {
	return ds.dao.Read(doc, uuid)
}

func (ds *DaoService) ReadRaw(uuid uuid.UUID) ([]byte, error) {
	return ds.dao.ReadRaw(uuid)
}
func (ds *DaoService) Update(doc dao.Document) error {
	return ds.dao.Update(doc)
}

func (ds *DaoService) Delete(id uuid.UUID) error {
	return ds.dao.Delete(id)
}
