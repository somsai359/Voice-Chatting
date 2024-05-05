  import React, { useState, useEffect, useRef } from 'react'; // Importing useEffect and useRef
  import { useRouter } from 'next/router';

  const IndexPage = () => {
    const [username, setUsername] = useState('');
    const router = useRouter();
    const ws = useRef(null); // Define ws useRef

    const handleStartChat = () => {
      if (username.trim() !== '') {
        // Encode the username before passing it as a query parameter
        const encodedUsername = encodeURIComponent(username);
        // Redirect to the chat page with the username as a query parameter
        router.push(`/chat?username=${encodedUsername}`)
          .then(() => console.log('Navigation successful'))
          .catch(err => console.error(err));
      }
    };

    useEffect(() => {
      if (username) {
        const decodedUsername = decodeURIComponent(username);
        // setProfileUsername(decodedUsername); // This seems to be undefined in this component
        ws.current = new WebSocket('ws://localhost:8080/ws');
        ws.current.onopen = () => {
          ws.current.send(JSON.stringify({ type: 'join', username: decodedUsername }));
        };
        ws.current.onmessage = (event) => {
          const data = JSON.parse(event.data);
          // setConnectedUsers(data.users); // This seems to be undefined in this component
        };
        return () => {
          ws.current.close();
        };
      }
    }, [username, router]); // Added router as a dependency

    return (
      <div className="flex flex-col items-center justify-center min-h-screen" style={{ background: 'linear-gradient(to bottom, rgb(23, 105, 206), rgb(955, 9081, 937))' }}>
        <div className="flex flex-col items-center justify-center h-full">
          <h1 className="text-4xl font-bold text-white mb-8 animated-heading">Welcome to Voice Chat</h1>
          <div className="max-w-md w-full mx-auto rounded-lg overflow-hidden shadow-lg bg-white animated-form">
            <div className="p-6">
              <input 
                type="text" 
                placeholder="Enter your username" 
                className="w-full bg-gray-100 text-gray-800 border border-gray-300 rounded-lg px-4 py-3 mb-4 focus:outline-none focus:ring focus:border-blue-500"
                value={username} 
                onChange={(e) => setUsername(e.target.value)} 
              />
              <button 
                onClick={handleStartChat} 
                className="w-full bg-blue-500 text-white rounded-lg px-4 py-3 hover:bg-blue-600 focus:outline-none focus:ring focus:border-blue-500"
              >
                Start Voice Chat
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  };

  export default IndexPage;
