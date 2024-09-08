package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"strconv"
	"sync"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	host "github.com/libp2p/go-libp2p/core/host"
	net "github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"
)

/*
	Unidade atômica de uma blockchain, contém informações sobre a transação

Index: Índice único do bloco na chain.
Timestamp: String com o horário, data, fuso horário, etc.
Value: Valor da transação(?).
Hash: Hash única do bloco.
PrevHash: Hash do bloco imediatamente anterior.
*/
type Block struct {
	Index     int
	Timestamp string
	Value     float64
	Hash      string
	PrevHash  string
}

// Array de blocos que configura a blockchain.
var Blockchain []Block

// Mutex para sincronização futura.
var mutex = &sync.Mutex{}

// Método que verifica a validade do bloco, usando hashes entre blocos, hash do ultimo bloco e indice dos blocos.
func blocoEhValido(blocoNovo, blocoVelho Block) bool {
	if blocoNovo.Index != (blocoVelho.Index+1) || blocoNovo.PrevHash != blocoVelho.Hash || calculaHash(blocoNovo) != blocoNovo.Hash {
		return false
	}
	return true
}

// Método que calcula uma hash para o bloco.
func calculaHash(bloco Block) string {
	record := strconv.Itoa(bloco.Index) + bloco.Timestamp + strconv.FormatFloat(bloco.Value, 'f', 16, 64) + bloco.PrevHash
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

func geraBloco(blocoVelho Block, value float64) Block {
	var blocoNovo Block
	t := time.Now()

	blocoNovo.Index = blocoVelho.Index + 1
	blocoNovo.Timestamp = t.String()
	blocoNovo.Value = value
	blocoNovo.PrevHash = blocoVelho.Hash

	blocoNovo.Hash = calculaHash(blocoNovo)

	return blocoNovo
}

func makeBasicHost(porta int, secio bool, randseed int64) (host.Host, error) {
	// Se a seed for zero, use aletoriedade do Reader: uses the ProcessPrng API no Windows.
	// Senão, use aleatoriedade determinística para gerar sempre as mesmas chaves em runs diferentes.
	var r io.Reader
	if randseed == 0 {
		r = rand.Reader
	} else {
		r = mrand.New(mrand.NewSource(randseed))
	}

	// Gera um par de chaves para este host. Vamos usá-las para obter IDs válidos para o host.
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	// Checa erro na geração de chaves
	if err != nil {
		return nil, err
	}

	// Cria opções de configuração para o host: IP e chave privada gerada acima.
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", porta)),
		libp2p.Identity(priv),
	}

	// Se não houver segurança, adicione NoSecurity nas opções.
	if !secio {
		opts = append(opts, libp2p.NoSecurity)
	}

	// Cria o host com as configs definidas.
	basicHost, err := libp2p.New(opts...)
	// Checa por erros na criação do host.
	if err != nil {
		return nil, err
	}

	// Cria um multi endereço de acesso.
	endHost, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", basicHost.ID()))
	// Pega o endereço que o host está ouvindo.
	addr := basicHost.Addrs()[0]
	// Adiciona o multi endereço ao endereço acima.
	endCompleto := addr.Encapsulate(endHost)
	// Printa para debug
	log.Printf("Eu sou %s\n", endCompleto)

	// Se houver segurança, adiciona a tag no comando a ser executado em outro terminal.
	if secio {
		log.Printf("Now run \"go run main.go -l %d -d %s -secio\" on a different terminal\n", porta+1, endCompleto)
	} else {
		log.Printf("Now run \"go run main.go -l %d -d %s\" on a different terminal\n", porta+1, endCompleto)
	}

	return basicHost, nil
}

func handleStream(s net.Stream) {
	log.Println("Chegou uma stream")

	// Cria buffer para escrita não bloqueante
	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	go readData(rw)
	go writeData(rw)
	// Stream s ficará aberta até seu fechamento.
}

/* Função de leitura:
 */
func readData(rw *bufio.ReadWriter) {

	//loop infinito pois precisa ficar aberta para receber possíveis chains.
	for {
		// Lemos a entrada do buffer que vem de outro nó.
		str, err := rw.ReadString('\n')
		// Checamos para erro.
		if err != nil {
			log.Fatal(err)
		}
		// Se o buffer estiver vazio, não processa.
		if str == "" {
			return
		}
		// Se o buffer não estiver vazio...
		if str != "\n" {
			// ... cria uma nova var de blockchain.
			chain := make([]Block, 0)
			// Abrimos o json da entrada e colocamos no objeto chain criado, checamos para erro.
			if err := json.Unmarshal([]byte(str), &chain); err != nil {
				log.Fatal(err)
			}
			// Usamos o mutex criado para bloquear o acesso ao uso do recurso.
			mutex.Lock()
			// Se a chain recebida for maior que a atual deste nó...
			if len(chain) > len(Blockchain) {
				// ... atualizamos a chain deste bloco.
				Blockchain = chain
				// Reindentamos para o formato json.
				bytes, err := json.MarshalIndent(Blockchain, "", " ")
				// Checamos para erro.
				if err != nil {
					log.Fatal(err)
				}
				// Green console color: 	\x1b[32m
				// Reset console color: 	\x1b[0m
				fmt.Printf("\x1b[32m%s\x1b[0m> ", string(bytes))
			}
			// Liberamos para uso a chain.
			mutex.Unlock()
		}
	}
}

func writeData(){

}
