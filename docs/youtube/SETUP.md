# Ligar o Tribuna ao YouTube

Faz-se uma vez, em duas partes:

- **Parte 1, em qualquer computador**: criar o cliente OAuth na Google Cloud. Serve qualquer conta Google, a sua incluída. No fim tem um ID e um segredo, que leva para o PC do clube (numa pen, por exemplo).
- **Parte 2, no PC do clube**: no painel do Tribuna, na **Configuração inicial**, colar o ID e o segredo e entrar com a conta que gere o canal do clube.

Com o YouTube ligado, o Tribuna cria o direto de cada jogo com o título e a miniatura, põe-no público enquanto dura e, no fim, torna o vídeo não listado e mostra o link para mandar aos jogadores.

Os botões da Google Cloud estão escritos aqui como aparecem com a consola em inglês. Se a consola estiver em português, os nomes mudam mas os passos são os mesmos.

## Parte 1. Google Cloud (qualquer computador, qualquer conta Google)

### A. Criar o projeto

1. Abra https://console.cloud.google.com e entre com a sua conta Google.
2. No topo, carregue no seletor de projetos e depois em **New project**.
3. Em **Project name**, escreva `Tribuna`. Carregue em **Create** e espere que o projeto fique escolhido no topo.
4. Abra https://console.cloud.google.com/apis/library/youtube.googleapis.com e carregue em **Enable**.

### B. Ecrã de consentimento (o passo que evita que o login caduque)

5. Abra https://console.cloud.google.com/auth/overview e carregue em **Get started**.
6. Em **App Information**, escreva `Tribuna` no nome e escolha o seu email em **User support email**. Carregue em **Next**.
7. Em **Audience**, escolha **External**. Carregue em **Next**.
8. Em **Contact Information**, escreva o seu email. Carregue em **Next**, aceite a política e carregue em **Create**.
9. Abra **Branding** (https://console.cloud.google.com/auth/branding). O Google só deixa publicar a app com estes campos preenchidos:
   - **Application home page** e **Application privacy policy link**: o endereço de uma página de privacidade no site do clube (por exemplo `https://www.clube-exemplo.pt/privacidade/`). Basta dizer que a app só usa o acesso ao YouTube para criar e gerir os diretos do clube e que não guarda dados pessoais.
   - **Authorized domains**: o domínio desse site (no exemplo, `clube-exemplo.pt`)

   Confirme que o nome e o email de suporte estão lá e carregue em **Save**.
10. Abra **Audience** (https://console.cloud.google.com/auth/audience). Em **Publishing status**, carregue em **Publish app** e confirme. O estado tem de ficar **In production**.

Com a app em **Testing**, o Google deixa o login durar só 7 dias, e o Tribuna pediria para ligar outra vez todas as semanas. Em **In production**, o login dura até alguém o cancelar, até ficar 6 meses sem uso ou até carregar em Desligar.

A app não precisa de ser verificada pelo Google. É só para o clube (menos de 100 contas), e o Google deixa entrar depois de um aviso. Não junte nada em **Data Access**. O Tribuna pede a permissão do YouTube sozinho, ao ligar.

### C. Criar o cliente OAuth

11. Abra **Clients** (https://console.cloud.google.com/auth/clients) e carregue em **Create client**.
12. Em **Application type**, escolha **Desktop app**. Em **Name**, escreva `Tribuna`. Carregue em **Create**.
13. Aparece o **Client ID** (acaba em `.apps.googleusercontent.com`) e o **Client secret**. Carregue em **Download JSON**, ou copie os dois para um ficheiro de texto. O Google pode não voltar a mostrar o segredo depois de fechar esta janela.
14. Ponha esse ficheiro numa pen e leve-o para o PC do clube. Não mande o ID nem o segredo por email ou WhatsApp. Não o deixe nas Transferências nem numa pasta sincronizada (OneDrive, Google Drive). Depois de ligar, apague o ficheiro da pen.

### D. Dar o projeto também ao clube (opcional)

Para o clube não depender da sua conta, junte a conta do clube como dona do projeto: em https://console.cloud.google.com/iam-admin/iam, carregue em **Grant access**, escreva o email da conta do clube, escolha o papel **Owner** e carregue em **Save**.

## Parte 2. No PC do clube

15. Abra o painel do Tribuna no PC do clube (não no telemóvel). Em **Ecrãs**, carregue em **Começar** no aviso "Falta a configuração inicial", ou vá a **Definições > Configuração inicial**.
16. No passo **YouTube**, cole o **ID do cliente** e o **Segredo do cliente**. Carregue em **Guardar**.
17. Carregue em **Ligar ao YouTube**. Abre-se a página do Google.
18. Entre com a conta que gere o canal do clube. Se o Google perguntar que canal usar, escolha o canal do clube.
19. O Google avisa que a app não foi verificada. Carregue em **Advanced** (Avançadas) e depois em **Go to Tribuna (unsafe)**. A app é a do clube, criada na parte 1.
20. Carregue em **Continue** para dar acesso ao canal. A página diz "Ligado ao YouTube". Feche-a e volte ao painel.
21. O passo mostra o nome e a imagem do canal. O Tribuna cria a transmissão "Tribuna" no YouTube e põe o OBS a enviar para ela.

### Que conta usar no passo 18

Tem de ser a conta Google dona do canal, ou uma conta dona ou gestora da conta de marca (Brand Account) do canal. Um convite em **Permissões** no YouTube Studio (Gestor, Editor e outros) não chega. O YouTube diz que essas contas "can't manage via YouTube APIs" (https://support.google.com/youtube/answer/9367690). A conta da parte 1 não tem de ser a mesma.

### No canal do YouTube (passo "Canal do YouTube")

22. O canal tem de ter os diretos ligados: https://www.youtube.com/features. Na primeira vez, o YouTube pode demorar até 24 horas a ligá-los.
23. Para a miniatura de cada jogo aparecer, o canal tem de estar verificado com o telemóvel: https://www.youtube.com/verify. Sem isso o direto começa na mesma, com uma imagem escolhida pelo YouTube, e o painel avisa.

Com o YouTube ligado, o painel pergunta ao YouTube se as duas coisas estão feitas. Sem ele, marque **Já fiz**.

O Tribuna guarda o login na pasta `dados`, no ficheiro `youtube.json`, cifrado para o utilizador do Windows que o abriu. Use sempre o mesmo utilizador do Windows. Se copiar a pasta para outro PC ou outro utilizador, o login não abre lá e é preciso carregar outra vez em **Ligar ao YouTube**. Ponha o programa numa pasta a que só a conta do clube tenha acesso, como `C:\Tribuna` num PC só do clube ou a pasta do próprio utilizador.

## Se algo correr mal

- **O Google não deixa carregar em Publish app**: falta algum campo do passo 9. Abra **Branding** outra vez e confirme o nome, o email de suporte, os dois links e o domínio.
- **O Google recusa o domínio**: o domínio em **Authorized domains** tem de ser o mesmo dos dois links do passo 9, sem `https://` nem `www.`.
- **"A ligação ao YouTube expirou"**: na Configuração inicial ou em Definições > YouTube, carregue em **Ligar ao YouTube** outra vez. Se acontecer todas as semanas, a app ainda está em **Testing**: repita o passo 10.
- **"O Google não aceitou o ID ou o segredo"**: copie-os outra vez do passo 13. Se o segredo já não aparecer, em **Clients** abra o cliente e crie um segredo novo.
- **O login entra, mas no canal errado**: carregue em **Desligar** e ligue outra vez, escolhendo o canal do clube.
- **Desligar**: o Tribuna cancela o acesso no Google e apaga o login do computador. Os vídeos ficam no canal como estão.

## Quanto gasta da quota do YouTube

O YouTube dá 10 000 unidades por dia a cada projeto. Um jogo gasta no máximo cerca de 115: criar o direto e ligá-lo à transmissão (2), a miniatura (50), ver se o direto acabou (até 10 vezes, 1 cada), acabá-lo se ficar preso (1) e torná-lo não listado (51). O separador Transmissões lê o canal no máximo de 10 em 10 minutos (1 ou 2 unidades). Abrir a Configuração inicial pergunta pelo canal (2).
