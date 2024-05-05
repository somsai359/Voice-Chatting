import React, { useState, useEffect, useRef } from 'react';
import { useRouter } from 'next/router';

const ChatPage = () => {
  const [recording, setRecording] = useState(false);
  const [recorder, setRecorder] = useState(null);
  const [connectedUsers, setConnectedUsers] = useState([]);
  const [profileUsername, setProfileUsername] = useState('');
  const [timer, setTimer] = useState(0);
  const [audioURL, setAudioURL] = useState('');
  const [muted, setMuted] = useState(false);
  const [stream, setStream] = useState(null);
  const [clickedProfileIndex, setClickedProfileIndex] = useState(null); // New state variable
  const router = useRouter();
  const { username } = router.query;
  const ws = useRef(null);
  const mediaStream = useRef(null);

  useEffect(() => {
    let interval;
    if (recording) {
      interval = setInterval(() => {
        setTimer(prevTimer => prevTimer + 1);
      }, 1000);
    } else {
      clearInterval(interval);
      setTimer(0);
    }
    return () => clearInterval(interval);
  }, [recording]);

  useEffect(() => {
    const getUserMedia = async () => {
      try {
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
        setStream(stream);
      } catch (error) {
        console.error('Error accessing microphone:', error);
      }
    };

    getUserMedia();

    return () => {
      if (stream) {
        stream.getTracks().forEach(track => track.stop());
      }
    };
  }, []);

  const startRecording = () => {
    if (!stream) return;
    try {
      const chunks = [];
      const mediaRecorder = new MediaRecorder(stream);
      mediaRecorder.ondataavailable = (e) => {
        if (e.data.size > 0) {
          chunks.push(e.data);
        }
      };
      mediaRecorder.onstop = () => {
        const blob = new Blob(chunks, { type: 'audio/wav' });
        setRecorder(null);
        setRecording(false);
        setAudioURL(URL.createObjectURL(blob));
      };
      mediaRecorder.start();
      setRecorder(mediaRecorder);
      setRecording(true);
    } catch (error) {
      console.error('Error starting recording:', error);
    }
  };

  const stopRecording = () => {
    if (recorder) {
      recorder.stop();
    }
  };

  const toggleMute = () => {
    if (stream) {
      stream.getAudioTracks().forEach(track => {
        track.enabled = !muted;
      });
      setMuted(!muted);
    }
  };

  const handleExit = () => {
    router.push('/');
  };

  const downloadAudio = () => {
    if (audioURL) {
      const link = document.createElement('a');
      link.href = audioURL;
      link.download = `recorded_audio_${new Date().toISOString()}.wav`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
    }
  };

  useEffect(() => {
    if (username) {
      setProfileUsername(username);
      ws.current = new WebSocket('ws://localhost:8080/ws');
      ws.current.onopen = () => {
        ws.current.send(JSON.stringify({ type: 'join', username }));
      };
      ws.current.onmessage = (event) => {
        const data = JSON.parse(event.data);
        if (data.type === 'userUpdate') {
          setConnectedUsers(data.users);
        }
      };
      return () => {
        ws.current.close();
      };
    }
  }, [username]);

  const calculateCircleSize = (index) => {
    return clickedProfileIndex === index ? 100 : 60; // Larger size if the index matches clickedProfileIndex
  };

  const handleProfileClick = (index) => {
    setClickedProfileIndex(prevIndex => {
      // Toggle the clicked profile's size
      return prevIndex === index ? null : index;
    });
  };

  return (
    <div className="min-h-screen bg-gradient-to-b from-blue-900 via-transparent to-transparent bg-opacity-50 flex flex-col justify-center items-center">
      <div className="bg-white bg-opacity-50 shadow-lg rounded-lg p-6 w-full max-w-xl">
        <h1 className="text-2xl font-semibold mb-4 text-center text-blue-gray-900">Voice Chat</h1>
        <div className="flex justify-between mb-4">
          {connectedUsers.length > 0 && (
            <span className="text-sm text-blue-gray-900">Online: {connectedUsers.length}</span>
          )}

          {profileUsername === username && (
            <>
              {!recording ? (
                <button onClick={startRecording} className="bg-green-500 text-white px-4 py-2 rounded-md font-semibold focus:outline-none hover:bg-green-600 transition duration-300 mr-2">Start Recording</button>
              ) : (
                <>
                  <span className="text-white">Recording: {timer} sec</span>
                  <button onClick={stopRecording} className="bg-red-500 text-white px-4 py-2 rounded-md font-semibold focus:outline-none hover:bg-red-600 transition duration-300 mr-2">Stop Recording</button>
                </>
              )}
              <button onClick={toggleMute} className={`bg-${muted ? 'red' : 'green'}-500 text-white px-4 py-2 rounded-md font-semibold focus:outline-none hover:bg-${muted ? 'red' : 'green'}-600 transition duration-300 mr-2`}>{muted ? 'Unmute' : 'Mute'}</button>
            </>
          )}
          {audioURL && (
            <button onClick={downloadAudio} className="bg-blue-500 text-white px-4 py-2 rounded-md font-semibold focus:outline-none hover:bg-blue-600 transition duration-300 mr-2">
              Download Audio
            </button>
          )}
        </div>
        <div className="flex flex-col items-center">
          <div className="bg-gray-200 text-gray-800 px-4 py-2 rounded-full flex items-center justify-center mb-4" style={{ width: '60px', height: '60px' }}>
            <span className="font-semibold">{profileUsername}</span>
          </div>
          <div className="flex space-x-4">
            {connectedUsers.map((user, index) => (
              <div key={index} className="bg-gray-300 px-4 py-2 rounded-full cursor-pointer hover:bg-gray-400 transition duration-300" onClick={() => handleProfileClick(index)} style={{ width: `${calculateCircleSize(index)}px`, height: `${calculateCircleSize(index)}px` }}>
                <span className="text-gray-800">{user}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};

export default ChatPage;
