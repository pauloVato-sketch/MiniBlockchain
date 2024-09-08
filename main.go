package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	spew "github.com/davecgh/go-spew/spew"
	golog "github.com/ipfs/go-log"
	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	host "github.com/libp2p/go-libp2p/core/host"
	net "github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	pstore "github.com/libp2p/go-libp2p/core/peerstore"
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
		// If the channel is closed or we get an EOF, return
		if err == io.EOF {
			return
		}
		// Se o buffer estiver vazio, não processa.
		if str == "" {
			continue
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

// Função com intuito de avisar aos nós conectados se adicionarmos um bloco na nossa chain para que seja aceito.
func writeData(rw *bufio.ReadWriter) {
	// Cria uma corrotina*
	go func() {
		for {
			// Aguarda 5 segundos
			time.Sleep(5 * time.Second)
			// Trava para realizar a operação de extração da chain
			mutex.Lock()
			bytes, err := json.Marshal(Blockchain)
			// Checa para erro
			if err != nil {
				log.Println(err)
			}
			// Libera trava.
			mutex.Unlock()

			// Trava para escrita.
			mutex.Lock()
			// Escreve a string (dados da blockchain) no buffer.
			rw.WriteString(fmt.Sprintf("%s\n", string(bytes)))
			// Limpa o buffer
			rw.Flush()
			// Libera o recurso.
			mutex.Unlock()
		}
		// * A cada 5 segundos, extrai e escreve a chain no buffer.
	}()

	// Cria buffer p/ escrita de um bloco.
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		// Lê buffer até a quebra de linha.
		sendData, err := stdReader.ReadString('\n')
		// Checa para erro
		if err != nil {
			log.Fatal(err)
		}
		// Substitui a quebra de linha com vazio.
		sendData = strings.Replace(sendData, "\r\n", "", -1)
		log.Printf("%s\n", sendData)
		// Extrai valor do buffer convertendo para float.
		value, err := strconv.ParseFloat(sendData, 64)
		// Checa para erro
		if err != nil {
			log.Fatal(err)
		}

		// Cria bloco com o valor digitado.
		novoBloco := geraBloco(Blockchain[len(Blockchain)-1], value)

		// Se bloco for valido, adiciona ele à blockchain.
		if blocoEhValido(novoBloco, Blockchain[len(Blockchain)-1]) {
			mutex.Lock()
			Blockchain = append(Blockchain, novoBloco)
			mutex.Unlock()
		}

		// Coloca blockchain no formato JSON
		bytes, err := json.Marshal(Blockchain)
		// Checa para erro
		if err != nil {
			log.Println(err)
		}
		// Printa a chain de forma interpretavel
		spew.Dump(Blockchain)

		// Pega a trava, escreve no buffer a nova blockchain em json, limpa buffer e libera o recurso.
		mutex.Lock()
		rw.WriteString(fmt.Sprintf("%s\n", string(bytes)))
		rw.Flush()
		mutex.Unlock()

	}
}

func main() {
	t := time.Now()
	// Cria e insere bloco base na chain.
	genesisBlock := Block{}
	genesisBlock = Block{0, t.String(), 0, calculaHash(genesisBlock), ""}

	Blockchain = append(Blockchain, genesisBlock)

	golog.SetAllLoggers(golog.LevelInfo)
	// Define flags
	/*
		listenF: Abre a porta na qual queremos receber as conexões (agindo como host).
		target: Especifica o endereço do alvo no qual queremos nos conectar (agindo como peer).
		secio: Opção de ter ou não segurança nas streams.
		seed: seed para aleatoriedade.
	*/
	listenF := flag.Int("l", 0, "aguarda por futuras conexões")
	target := flag.String("d", "", "nó alvo para \"discar\"")
	secio := flag.Bool("secio", false, "habilitar secio")
	seed := flag.Int64("seed", 0, "define seed aleatória para geração de IDs")
	flag.Parse()
	if *listenF == 0 {
		log.Fatal("Digite a porta para bind com -l")
	}

	// Cria o host para ouvir o multi endereço
	host, err := makeBasicHost(*listenF, *secio, *seed)
	if err != nil {
		log.Fatal(err)
	}
	// Se o target está vazio, estamos agindo apenas como host
	if *target == "" {
		log.Println("Aguardando por conexões...")

		// Coloca um handler no host A. /p2p/1.0.0 é um nome de protocolo definido pelo usuário.
		host.SetStreamHandler("/p2p/1.0.0", handleStream)

		select {} // Fica aqui para sempre ?
	} else {
		host.SetStreamHandler("/p2p/1.0.0", handleStream)

		// Extrai IP do nó alvo
		ipfsAddr, err := ma.NewMultiaddr(*target)
		if err != nil {
			log.Fatalln(err)
		}

		pid, err := ipfsAddr.ValueForProtocol(ma.P_IPFS)
		if err != nil {
			log.Fatalln(err)
		}

		peerId, err := peer.Decode(pid)
		log.Println(fmt.Printf("\x1b[32m%s\x1b[0m> ", (peerId)))
		if err != nil {
			log.Fatalln(err)
		}
		// Desencapsula a parte /ipfs/<peerID> do alvo
		// /ip4/<a.b.c.d>/ipfs/<peer> vira /ip4/<a.b.c.d>

		endPeerAlvo, _ := ma.NewMultiaddr(
			fmt.Sprintf("/ipfs/%s", peer.ID.String(peerId)))
		endAlvo := ipfsAddr.Decapsulate(endPeerAlvo)

		// Temos um id de peer e um endereço alvo, então adicionamos para a peerstore para a lib entender como alcançá-lo.
		host.Peerstore().AddAddr(peerId, endAlvo, pstore.PermanentAddrTTL)

		log.Println("Abrindo stream...")
		// Cria stream do host B para o host A
		s, err := host.NewStream(context.Background(), peerId, "/p2p/1.0.0")
		if err != nil {
			log.Fatalln(err)
		}
		// Cria uma stream movida à buffer para que escritas e leituras sejam não bloqueantes.
		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

		go writeData(rw)
		go readData(rw)

		select {}
	}
}
