package fdfs_client

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
)

type storageUploadTask struct {
	header
	//req
	fileInfo         *fileInfo
	storagePathIndex int8
	//res
	fileId string
}

func (this *storageUploadTask) SendReq(conn net.Conn) error {
	this.cmd = STORAGE_PROTO_CMD_UPLOAD_FILE
	this.pkgLen = this.fileInfo.fileSize + 15

	if err := this.SendHeader(conn); err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	buffer.WriteByte(byte(this.storagePathIndex))
	if err := binary.Write(buffer, binary.BigEndian, this.fileInfo.fileSize); err != nil {
		return err
	}

	byteFileExtName := []byte(this.fileInfo.fileExtName)
	var bufferFileExtName [6]byte
	for i := 0; i < len(byteFileExtName); i++ {
		bufferFileExtName[i] = byteFileExtName[i]
	}
	buffer.Write(bufferFileExtName[:])

	if _, err := conn.Write(buffer.Bytes()); err != nil {
		return err
	}

	var err error
	//send file
	if this.fileInfo.file != nil {
		_, err = conn.(pConn).Conn.(*net.TCPConn).ReadFrom(this.fileInfo.file)
	} else {
		_, err = conn.Write(this.fileInfo.buffer)
	}

	if err != nil {
		return err
	}
	return nil
}

func (this *storageUploadTask) RecvRes(conn net.Conn) error {
	if err := this.RecvHeader(conn); err != nil {
		return err
	}

	if this.pkgLen <= 16 {
		return fmt.Errorf("recv file id pkgLen <= FDFS_GROUP_NAME_MAX_LEN")
	}
	if this.pkgLen > 100 {
		return fmt.Errorf("recv file id pkgLen > 100,can't be so long")
	}

	buf := make([]byte, this.pkgLen)
	if _, err := conn.Read(buf); err != nil {
		return err
	}

	buffer := bytes.NewBuffer(buf)
	groupName, err := readCStrFromByteBuffer(buffer, 16)
	if err != nil {
		return err
	}
	remoteFileName, err := readCStrFromByteBuffer(buffer, int(this.pkgLen)-16)
	if err != nil {
		return err
	}

	this.fileId = groupName + "/" + remoteFileName
	return nil
}

type storageDownloadTask struct {
	header
	//req
	groupName      string
	remoteFilename string
	offset         int64
	downloadBytes  int64
	//res
	localFilename string
	buffer        []byte
}

func (this *storageDownloadTask) SendReq(conn net.Conn) error {
	this.cmd = STORAGE_PROTO_CMD_DOWNLOAD_FILE
	this.pkgLen = int64(len(this.remoteFilename) + 32)

	if err := this.SendHeader(conn); err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	if err := binary.Write(buffer, binary.BigEndian, this.offset); err != nil {
		return err
	}
	if err := binary.Write(buffer, binary.BigEndian, this.downloadBytes); err != nil {
		return err
	}
	byteGroupName := []byte(this.groupName)
	var bufferGroupName [16]byte
	for i := 0; i < len(byteGroupName); i++ {
		bufferGroupName[i] = byteGroupName[i]
	}
	buffer.Write(bufferGroupName[:])
	buffer.WriteString(this.remoteFilename)
	if _, err := conn.Write(buffer.Bytes()); err != nil {
		return err
	}

	return nil
}

func (this *storageDownloadTask) RecvRes(conn net.Conn) error {
	if err := this.RecvHeader(conn); err != nil {
		return fmt.Errorf("StorageDownloadTask RecvRes %v", err)
	}
	if this.localFilename != "" {
		if err := this.recvFile(conn); err != nil {
			return fmt.Errorf("StorageDownloadTask RecvRes %v", err)
		}
	} else {
		if err := this.recvBuffer(conn); err != nil {
			return fmt.Errorf("StorageDownloadTask RecvRes %v", err)
		}
	}
	return nil
}

func (this *storageDownloadTask) recvFile(conn net.Conn) error {
	file, err := os.Create(this.localFilename)
	defer file.Close()
	if err != nil {
		return err
	}

	writer := bufio.NewWriter(file)

	if err := writeFromConn(conn, writer, this.pkgLen); err != nil {
		return fmt.Errorf("StorageDownloadTask RecvFile %s", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("StorageDownloadTask RecvFile %s", err)
	}
	return nil
}

func (this *storageDownloadTask) recvBuffer(conn net.Conn) error {
	var (
		err error
	)
	//buffer allocate by user
	if this.buffer != nil {
		if int64(len(this.buffer)) < this.pkgLen {
			return fmt.Errorf("StorageDownloadTask buffer < pkgLen can't recv")
		}
		if err = writeFromConnToBuffer(conn, this.buffer, this.pkgLen); err != nil {
			return fmt.Errorf("StorageDownloadTask writeFromConnToBuffer %s", err)
		}
		return nil
	}
	writer := new(bytes.Buffer)

	if err = writeFromConn(conn, writer, this.pkgLen); err != nil {
		return fmt.Errorf("StorageDownloadTask RecvBuffer %s", err)
	}
	this.buffer = writer.Bytes()
	return nil
}

type storageDeleteTask struct {
	header
	//req
	groupName      string
	remoteFilename string
}

func (this *storageDeleteTask) SendReq(conn net.Conn) error {
	this.cmd = STORAGE_PROTO_CMD_DELETE_FILE
	this.pkgLen = int64(len(this.remoteFilename) + 16)

	if err := this.SendHeader(conn); err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	byteGroupName := []byte(this.groupName)
	var bufferGroupName [16]byte
	for i := 0; i < len(byteGroupName); i++ {
		bufferGroupName[i] = byteGroupName[i]
	}
	buffer.Write(bufferGroupName[:])
	buffer.WriteString(this.remoteFilename)
	if _, err := conn.Write(buffer.Bytes()); err != nil {
		return err
	}
	return nil
}

func (this *storageDeleteTask) RecvRes(conn net.Conn) error {
	return this.RecvHeader(conn)
}

type storageUploadSlaveTask struct {
	header
	//req
	fileInfo         *fileInfo
	storagePathIndex int8
	//res
	fileId string
	// slave
	masterFilename string
	prefixName     string
	fileExtName    string
}

func (this *storageUploadSlaveTask) SendReq(conn net.Conn) error {
	this.cmd = STORAGE_PROTO_CMD_UPLOAD_SLAVE_FILE
	masterFilenameLen := int64(len(this.masterFilename))
	// #slave_fmt |-master_len(8)-file_size(8)-prefix_name(16)-file_ext_name(6)
	//       #           -master_name(master_filename_len)-|
	headerLen := int64(38) + masterFilenameLen

	this.pkgLen = this.fileInfo.fileSize + headerLen

	if err := this.SendHeader(conn); err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	binary.Write(buffer, binary.BigEndian, masterFilenameLen)
	if err := binary.Write(buffer, binary.BigEndian, this.fileInfo.fileSize); err != nil {
		return err
	}

	// 16 bit prefixName
	prefixNameBytes := bytes.NewBufferString(this.prefixName).Bytes()
	for i := 0; i < 16; i++ {
		if i >= len(prefixNameBytes) {
			buffer.WriteByte(byte(0))
		} else {
			buffer.WriteByte(prefixNameBytes[i])
		}
	}

	// 6 bit fileExtName
	fileExtNameBytes := bytes.NewBufferString(this.fileExtName).Bytes()
	for i := 0; i < 6; i++ {
		if i >= len(fileExtNameBytes) {
			buffer.WriteByte(byte(0))
		} else {
			buffer.WriteByte(fileExtNameBytes[i])
		}
	}

	// master_filename_len bit master_name
	masterFilenameBytes := bytes.NewBufferString(this.masterFilename).Bytes()
	for i := 0; i < int(masterFilenameLen); i++ {
		buffer.WriteByte(masterFilenameBytes[i])
	}

	if _, err := conn.Write(buffer.Bytes()); err != nil {
		return err
	}

	var err error
	//send file
	if this.fileInfo.file != nil {
		_, err = conn.(pConn).Conn.(*net.TCPConn).ReadFrom(this.fileInfo.file)
	} else {
		_, err = conn.Write(this.fileInfo.buffer)
	}

	if err != nil {
		return err
	}
	return nil
}

func (this *storageUploadSlaveTask) RecvRes(conn net.Conn) error {
	if err := this.RecvHeader(conn); err != nil {
		return err
	}

	if this.pkgLen <= 16 {
		return fmt.Errorf("recv file id pkgLen <= FDFS_GROUP_NAME_MAX_LEN")
	}
	if this.pkgLen > 100 {
		return fmt.Errorf("recv file id pkgLen > 100,can't be so long")
	}

	buf := make([]byte, this.pkgLen)
	if _, err := conn.Read(buf); err != nil {
		return err
	}

	buffer := bytes.NewBuffer(buf)
	groupName, err := readCStrFromByteBuffer(buffer, 16)
	if err != nil {
		return err
	}
	remoteFileName, err := readCStrFromByteBuffer(buffer, int(this.pkgLen)-16)
	if err != nil {
		return err
	}

	this.fileId = groupName + "/" + remoteFileName
	return nil
}

type storageQueryFileInfoTask struct {
	header
	//req
	groupName      string
	remoteFilename string
	fileInfo       *FileInfo
	buffer         []byte
}

func (this *storageQueryFileInfoTask) SendReq(conn net.Conn) error {
	this.cmd = STORAGE_PROTO_CMD_QUERY_FILE_INFO
	this.pkgLen = int64(len(this.remoteFilename) + 16)

	if err := this.SendHeader(conn); err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	byteGroupName := []byte(this.groupName)
	var bufferGroupName [16]byte
	for i := 0; i < len(byteGroupName); i++ {
		bufferGroupName[i] = byteGroupName[i]
	}
	buffer.Write(bufferGroupName[:])
	buffer.WriteString(this.remoteFilename)
	if _, err := conn.Write(buffer.Bytes()); err != nil {
		return err
	}
	return nil
}

func (this *storageQueryFileInfoTask) RecvRes(conn net.Conn) error {
	if err := this.RecvHeader(conn); err != nil {
		return err
	}
	this.buffer = make([]byte, this.pkgLen)
	if err := writeFromConnToBuffer(conn, this.buffer, this.pkgLen); err != nil {
		return fmt.Errorf("QueryFileInfo writeFromConnToBuffer %s", err)
	}
	var (
		x               int32
		createTimeStamp int32
		crc32           int32
		fileSize        int64
	)
	buff := bytes.NewBuffer(this.buffer)
	binary.Read(buff, binary.BigEndian, &fileSize)
	binary.Read(buff, binary.BigEndian, &x)
	binary.Read(buff, binary.BigEndian, &createTimeStamp)
	binary.Read(buff, binary.BigEndian, &x)
	binary.Read(buff, binary.BigEndian, &crc32)
	ipAddr, err := readCStrFromByteBuffer(buff, 16-1)
	if err != nil {
		return err
	}
	this.fileInfo = &FileInfo{
		CreateTimeStamp: createTimeStamp,
		CRC32:           crc32,
		SourceID:        0,
		FileSize:        fileSize,
		SourceIPAddress: ipAddr,
	}
	return nil
}
