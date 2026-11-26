# Duas ou três câmaras

Este guia é para um clube que quer filmar o jogo com mais de uma câmara. Com uma só câmara, chega o LEIA-ME.txt.

## Antes de começar

O Hallstream põe cada câmara numa cena sua do OBS, com o grafismo por cima. Em **Ecrãs** aparece um botão por câmara, e a que está no ar diz "No ar". Carregue noutro botão para mudar de câmara.

Pode ter até três câmaras. Cada uma pode ser:

- **Um telemóvel com o DroidCam**: uma aplicação no telemóvel que manda a imagem para o OBS, por wifi ou por cabo USB.
- **Uma câmara ligada ao PC**: uma webcam USB, ou uma câmara com saída HDMI ligada a uma placa de captura.
- **Um telemóvel sem aplicação (VDO.Ninja)**: o telemóvel abre uma página no navegador e manda a imagem por lá.

Instale o OBS 31 ou mais recente e, para o DroidCam, a aplicação no telemóvel e o plugin DroidCam OBS no PC (https://droidcam.app/obs/).

## Escolher as câmaras

    Câmara                 Aplicação  Internet  Cabo   Custo
    DroidCam               sim        não       USB    HD só no Pro
    Câmara ligada ao PC    não        não       sempre o da câmara
    Telemóvel (VDO.Ninja)  não        sim       não    grátis

- **DroidCam**: a versão grátis fica em 480p. Para 720p ou 1080p sem marca de água é preciso comprar o DroidCam Pro, na aplicação do telemóvel. Para vários telemóveis, cada um é uma fonte DroidCam no OBS.
- **Câmara ligada ao PC**: não usa a rede. É a mais estável.
- **Telemóvel (VDO.Ninja)**: não instala nada, mas precisa de internet no PC e no telemóvel durante o jogo. O resto do Hallstream funciona sem internet.

## Quanto a rede aguenta

Cada telemóvel por wifi ocupa parte do wifi do pavilhão. Estes são os números das páginas do DroidCam e do VDO.Ninja, por câmara:

    Por câmara                            Mbps
    DroidCam 480p (versão grátis)         0,6 a 1,4
    DroidCam 720p, qualidade Normal       4
    DroidCam 1080p, qualidade Normal      7
    Telemóvel (VDO.Ninja)                 4, mais uma margem

O Hallstream pede ao VDO.Ninja 4000 kbps (4 Mbps) por telemóvel, sem som. O VDO.Ninja diz que o som e a "overhead" vêm por cima, sem dar um número. Conte com uma margem.

Somando as câmaras:

    Câmaras por wifi                      Duas    Três
    DroidCam 720p, Normal                 8       12
    DroidCam 1080p, Normal                14      21
    Telemóvel (VDO.Ninja)                 8+      12+

Por cima disto, o PC manda o direto para o YouTube. Para 1080p a 30 imagens por segundo, o YouTube recomenda 14 Mbps (H.264), com um mínimo de 5 Mbps. Esse envio usa a internet do pavilhão, e também o wifi se o PC estiver no wifi. Ligue o PC ao router por cabo de rede sempre que puder.

Uma regra: se a soma passar o que o wifi do pavilhão dá, ou se a imagem falhar no ensaio, ligue um telemóvel por cabo ou baixe uma câmara para 720p. O DroidCam recomenda 720p como o melhor equilíbrio entre qualidade e desempenho.

Se não sabe quanto dá o wifi, faça um ensaio antes do primeiro jogo, com todas as câmaras ligadas ao mesmo tempo durante uns minutos.

## Duas câmaras por wifi

1. Ligue o PC ao router com um cabo de rede, se puder.
2. Ligue os dois telemóveis à mesma rede wifi do PC. Se o router tem uma rede de 2,4 GHz e outra de 5 GHz, ponha todos na mesma.
3. Ponha cada telemóvel no carregador e num sítio onde o calor possa sair (não ao sol, não encostado a outra coisa quente). O DroidCam pede a bateria a 100% e ligada à corrente.
4. Abra o DroidCam nos dois telemóveis. Cada um mostra o seu endereço "WiFi IP".
5. No painel, abra **Definições > Câmaras**. Na **Câmara 1**, em **De onde vem a imagem**, escolha a fonte DroidCam. Em **Nome no painel**, escreva um nome curto, por exemplo `Bancada`.
6. Carregue em **Adicionar câmara** e escolha a fonte da **Câmara 2** da mesma maneira. Se o segundo telemóvel ainda não está no OBS, escolha o DroidCam em **Juntar ao OBS**.
7. Carregue em **Guardar e configurar o OBS**. Espere que a lista no cartão do OBS diga que está pronto.
8. No OBS, abra as propriedades de cada fonte DroidCam que ainda não tem imagem: marque **Use WiFi**, escreva o "WiFi IP" desse telemóvel e carregue em **Activate**.
9. Em **Ecrãs**, confirme que os dois botões dizem "Com imagem".

## Três câmaras por wifi

Faça os passos de cima para as três câmaras. Depois:

1. Veja a soma da secção "Quanto a rede aguenta". Três telemóveis em 1080p são 21 Mbps só de imagem.
2. Se a imagem falhar, baixe primeiro para 720p a câmara que menos vai para o ar (a do banco, por exemplo). No DroidCam muda-se nas definições da aplicação ou nas propriedades da fonte no OBS.
3. Se ainda falhar, ligue a câmara principal por cabo (secção seguinte).

## Com cabo

Um cabo tira essa câmara do wifi: a imagem vai pelo cabo. O DroidCam diz que por USB a ligação é mais estável, porque não há interferência nem perda de pacotes.

### Telemóvel Android com DroidCam, por USB

1. No telemóvel, abra as **Definições** e procure **Número de compilação** (Build Number), normalmente em **Acerca do telemóvel**.
2. Toque sete vezes em **Número de compilação**. Ficam ligadas as **Opções de programador**.
3. Volte às **Definições**, procure **Depuração USB** (USB Debugging) e ligue-a.
4. Ligue o telemóvel ao PC com o cabo USB. No telemóvel, aceite **Permitir depuração USB**.
5. Abra o DroidCam no telemóvel e, no painel, carregue em **Guardar e configurar o OBS**. O Hallstream liga a fonte DroidCam ao primeiro telemóvel USB que ainda não tem fonte.

### iPhone com DroidCam, por USB

1. Use um cabo original da Apple ou aprovado pela Apple.
2. No PC, confirme que está instalado o **Apple Mobile Device Support**: carregue na tecla Windows, procure **Aplicações instaladas** e veja se aparece na lista. Normalmente o Windows instala-o sozinho quando liga o iPhone.
3. Ligue o iPhone ao PC. Se o iPhone perguntar se confia neste computador, carregue em **Confiar**.
4. Abra o DroidCam no iPhone e, no painel, carregue em **Guardar e configurar o OBS**.

### Webcam ou placa de captura

Ligue-a a uma porta USB do PC. Em **Definições > Câmaras**, escolha-a em **De onde vem a imagem** (em **Juntar ao OBS**, se ainda não está no OBS) e carregue em **Guardar e configurar o OBS**.

### O PC por cabo de rede

Ligue o PC ao router com um cabo de rede. Os telemóveis por wifi continuam a chegar ao PC, desde que estejam no mesmo router.

### Uma mistura

Uma boa montagem: a câmara principal por cabo (DroidCam por USB ou uma câmara ligada ao PC) e as outras por wifi.

## Um telemóvel sem aplicação (VDO.Ninja)

Precisa de internet no PC e no telemóvel. O VDO.Ninja diz que a imagem fica normalmente na rede do pavilhão, mas abrir a página e ligar o telemóvel ao PC precisam de internet.

1. Em **Definições > Câmaras**, numa câmara, escolha **Telemóvel (VDO.Ninja)** em **De onde vem a imagem**.
2. Carregue em **Guardar e configurar o OBS**. Aparece um código QR nessa câmara.
3. No telemóvel que vai filmar, leia o código QR com a câmara do telemóvel e abra o link no navegador.
4. Deixe a página usar a câmara, quando o navegador perguntar.
5. Deite o telemóvel (na horizontal), ponha-o no carregador e deixe o ecrã ligado, com a página aberta.
6. Em **Ecrãs**, confirme que essa câmara diz "Com imagem".

Um código QR serve um telemóvel. Um segundo telemóvel no mesmo código é recusado pelo VDO.Ninja. Para outro telemóvel, junte outra câmara.

O código não muda: o mesmo telemóvel pode ler o mesmo código no jogo seguinte. Guarde-o como um segredo, porque quem tem o código pode mandar imagem para essa câmara.

## No dia do jogo

1. Ponha os telemóveis nos carregadores e abra o DroidCam, ou a página do VDO.Ninja, antes de abrir o Hallstream.
2. Em **Ecrãs**, veja a linha de cada câmara:
   - **Com imagem**: está tudo bem.
   - **Sem imagem**: o OBS não recebe nada dessa câmara, ou recebe uma imagem toda da mesma cor (preta, por exemplo). Confirme que a câmara está ligada e, num telemóvel, que o DroidCam ou a página do VDO.Ninja estão abertos.
   - **Imagem parada**: a imagem não mudou em 10 segundos. Num telemóvel, veja se o ecrã se desligou ou se a aplicação fechou.
3. Se aparecer "O OBS saltou … imagens no último minuto", o PC não está a dar conta. Veja a última alínea da secção seguinte.

Com uma só câmara não há botões: o cartão só aparece quando há um problema.

## Se algo correr mal

- **Um telemóvel DroidCam por wifi não dá imagem**: confirme que o telemóvel e o PC estão no mesmo router e, com redes de 2,4 GHz e 5 GHz, na mesma. Confirme que o "WiFi IP" na fonte do OBS é o que o telemóvel mostra. O DroidCam usa a porta 4747: uma firewall no PC ou no router pode bloqueá-la. Se nada resultar, ligue o telemóvel por USB.
- **Um telemóvel VDO.Ninja fica preto**: confirme que o PC e o telemóvel têm internet, que a página continua aberta no telemóvel e que o ecrã não bloqueou. Se fechou a página, leia o código QR outra vez.
- **"Sem imagem" numa câmara que está bem, só quando não está no ar**: no OBS, abra as propriedades da fonte. Numa câmara ligada ao PC, desligue **Deactivate when not showing**. Num vídeo ou fonte de teste, desligue **Restart playback when source becomes active**. Com estas opções ligadas, a fonte para quando a cena sai do ar e a linha diz "Sem imagem". Os nomes estão como aparecem no OBS em inglês; com o OBS em português, mudam. Nas fontes do VDO.Ninja, o Configurar OBS já deixa a opção certa.
- **O OBS saltou imagens**: o PC não consegue fazer tudo. Ponha uma câmara em 720p em vez de 1080p, feche os outros programas e, num portátil, ligue-o à corrente.

## Fontes

Lidas a 26 de novembro de 2026.

- Qualidades do DroidCam (Mbps por câmara), vários telemóveis, mesma rede e mesma banda, marca de água no HD sem Pro: https://droidcam.app/obs/usage/
- DroidCam recomenda 720p; USB mais estável; carregador e calor; porta 4747; depuração USB no Android; iPhone por USB: https://droidcam.app/help/
- A versão grátis do DroidCam fica em 480p: https://www.dev47apps.com/droidcam/hd-mode/
- VDO.Ninja, 4000 kbps em 1080p, cerca de 2500 kbps em 720p, som e overhead por cima: https://docs.vdo.ninja/guides/default-resolution-and-bitrate.md
- VDO.Ninja, o parâmetro `videobitrate` que o Hallstream usa (4000): https://docs.vdo.ninja/advanced-settings/video-bitrate-parameters/bitrate.md
- VDO.Ninja precisa de internet para abrir a página e ligar: https://docs.vdo.ninja/readme/how-does-it-work.md
- VDO.Ninja no OBS, "Shutdown source when not visible" desligado: https://docs.vdo.ninja/guides/enabling-webrtc-sources-in-obs.md
- YouTube, 14 Mbps recomendados para 1080p a 30 imagens por segundo (H.264): https://support.google.com/youtube/answer/2853702
